// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package csrctrl

import (
	"context"
	"os"
	"strings"
	"time"

	capi "k8s.io/api/certificates/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	// +kubebuilder:scaffold:imports
	"github.com/apache/dubbo-go-pixiu/pkg/test/csrctrl/signer"
	"github.com/apache/dubbo-go-pixiu/pkg/test/framework/components/cluster"
	"istio.io/pkg/log"
)

const (
	// Define the root path for signer to store CA and private key files.
	signerRoot = "/tmp/pki/signer/"

	// The duration of the signed certificates
	certificateDuration = 1 * time.Hour
)

var (
	scheme         = runtime.NewScheme()
	loggingOptions = log.DefaultOptions()
	_              = capi.AddToScheme(scheme)
	_              = corev1.AddToScheme(scheme)
)

type SignerRootCert struct {
	Signer   string
	Rootcert string
}

func RunCSRController(signerNames string, appendRootCert bool, c <-chan struct{},
	clusters cluster.Clusters,
) []SignerRootCert {
	// Config Istio log
	if err := log.Configure(loggingOptions); err != nil {
		log.Infof("Unable to configure Istio log error: %v", err)
		os.Exit(-1)
	}
	arrSigners := strings.Split(signerNames, ",")
	signersMap := make(map[string]*signer.Signer, len(arrSigners))
	var rootCertSignerArr []SignerRootCert
	for _, signerName := range arrSigners {
		signer, sErr := signer.NewSigner(signerRoot, signerName, certificateDuration)
		if sErr != nil {
			log.Infof("Unable to start signer for [%s], error: %v", signerName, sErr)
			os.Exit(-1)
		}
		signersMap[signerName] = signer
		rootCert, rErr := os.ReadFile(signer.GetRootCerts())
		if rErr != nil {
			log.Infof("Unable to read root cert for signer [%s], error: %v", signerName, sErr)
			os.Exit(-1)
		}
		rootCertsForSigner := SignerRootCert{
			Signer:   signerName,
			Rootcert: string(rootCert),
		}
		rootCertSignerArr = append(rootCertSignerArr, rootCertsForSigner)
	}

	for _, cluster := range clusters {
		mgr, err := ctrl.NewManager(cluster.RESTConfig(), ctrl.Options{
			Scheme: scheme,
			// disabel the metric server to avoid the port conflicting
			MetricsBindAddress: "0",
		})
		if err != nil {
			log.Infof("Unable to start manager error: %v", err)
			os.Exit(-1)
		}
		go runManager(mgr, arrSigners, signersMap, appendRootCert, c)
	}

	return rootCertSignerArr
}

func runManager(mgr manager.Manager, arrSigners []string, signersMap map[string]*signer.Signer, appendRootCert bool, c <-chan struct{}) {
	if err := (&CertificateSigningRequestSigningReconciler{
		Client:         mgr.GetClient(),
		SignerRoot:     signerRoot,
		CtrlCertTTL:    certificateDuration,
		Scheme:         mgr.GetScheme(),
		SignerNames:    arrSigners,
		Signers:        signersMap,
		appendRootCert: appendRootCert,
	}).SetupWithManager(mgr); err != nil {
		log.Infof("Unable to create Controller for controller CSRSigningReconciler, error: %v", err)
		os.Exit(-1)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-c
		cancel()
	}()
	// +kubebuilder:scaffold:builder
	log.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		log.Infof("Problem running manager, error: %v", err)
		os.Exit(-1)
	}
}
