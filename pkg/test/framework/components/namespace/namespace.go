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

package namespace

import (
	"time"

	"github.com/apache/dubbo-go-pixiu/pkg/test"
	"github.com/apache/dubbo-go-pixiu/pkg/test/framework/resource"
	"github.com/apache/dubbo-go-pixiu/pkg/test/scopes"
)

// Config contains configuration information about the namespace instance
type Config struct {
	// Prefix to use for autogenerated namespace name
	Prefix string
	// Inject indicates whether to add sidecar injection label to this namespace
	Inject bool
	// Revision is the namespace of custom injector instance
	Revision string
	// Labels to be applied to namespace
	Labels map[string]string
	// SkipDump, if enabled, will disable dumping the namespace. This is useful to avoid duplicate
	// dumping of istio-system.
	SkipDump bool
}

func (c *Config) overwriteRevisionIfEmpty(revision string) {
	// Overwrite the default namespace label (istio-injection=enabled)
	// with istio.io/rev=XXX. If a revision label is already provided,
	// the label will remain as is.
	if c.Revision == "" {
		c.Revision = revision
	}
	// Allow setting revision explicitly to `default` to avoid configuration overwrite
	if c.Revision == "default" {
		c.Revision = ""
	}
}

// Instance represents an allocated namespace that can be used to create config, or deploy components in.
type Instance interface {
	Name() string
	SetLabel(key, value string) error
	RemoveLabel(key string) error
	Prefix() string
	Labels() (map[string]string, error)
}

// Claim an existing namespace in all clusters, or create a new one if doesn't exist.
func Claim(ctx resource.Context, cfg Config) (i Instance, err error) {
	cfg.overwriteRevisionIfEmpty(ctx.Settings().Revisions.Default())
	return claimKube(ctx, cfg)
}

// ClaimOrFail calls Claim and fails test if it returns error
func ClaimOrFail(t test.Failer, ctx resource.Context, name string) Instance {
	t.Helper()
	nsCfg := Config{
		Prefix: name,
		Inject: true,
	}
	i, err := Claim(ctx, nsCfg)
	if err != nil {
		t.Fatalf("namespace.ClaimOrFail:: %v", err)
	}
	return i
}

// New creates a new Namespace in all clusters.
func New(ctx resource.Context, cfg Config) (i Instance, err error) {
	start := time.Now()
	scopes.Framework.Infof("=== BEGIN: Create namespace %s ===", cfg.Prefix)
	defer func() {
		if err != nil {
			scopes.Framework.Errorf("=== FAILED: Create namespace %s ===", cfg.Prefix)
			scopes.Framework.Error(err)
		} else {
			scopes.Framework.Infof("=== SUCCEEDED: Create namespace %s in %v ===", cfg.Prefix, time.Since(start))
		}
	}()

	if ctx.Settings().StableNamespaces {
		return Claim(ctx, cfg)
	}
	cfg.overwriteRevisionIfEmpty(ctx.Settings().Revisions.Default())
	return newKube(ctx, cfg)
}

// NewOrFail calls New and fails test if it returns error
func NewOrFail(t test.Failer, ctx resource.Context, nsConfig Config) Instance {
	t.Helper()
	i, err := New(ctx, nsConfig)
	if err != nil {
		t.Fatalf("namespace.NewOrFail: %v", err)
	}
	return i
}

// GetAll returns all namespaces that have exist in the context.
func GetAll(ctx resource.Context) ([]Instance, error) {
	var out []Instance
	if err := ctx.GetResource(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Setup is a utility function for creating a namespace in a test suite.
func Setup(ns *Instance, cfg Config) resource.SetupFn {
	return func(ctx resource.Context) (err error) {
		*ns, err = New(ctx, cfg)
		return
	}
}

// Getter for a namespace Instance
type Getter func() Instance

// Get is a utility method that helps in readability of call sites.
func (g Getter) Get() Instance {
	return g()
}

// Future creates a Getter for a variable that namespace that will be set at sometime in the future.
// This is helpful for configuring a setup chain for a test suite that operates on global variables.
func Future(ns *Instance) Getter {
	return func() Instance {
		return *ns
	}
}

func Dump(ctx resource.Context, name string) {
	ns := &kubeNamespace{
		ctx:    ctx,
		prefix: name,
		name:   name,
	}
	ns.Dump(ctx)
}

// NilGetter is a Getter that always returns nil.
var NilGetter = func() Instance {
	return nil
}
