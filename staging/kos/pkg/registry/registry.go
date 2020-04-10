/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package registry

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericregistry "k8s.io/apiserver/pkg/registry/generic/registry"
	"k8s.io/apiserver/pkg/registry/rest"
)

// REST implements rest.Storage and a bunch of other interfaces based
// on inheritance from the Store, plus CategoriesProvider and
// ShortNamesProvider --- all based on storing API objects in etcd.
type REST struct {
	*genericregistry.Store
	Categorys  []string
	ShortNamez []string
}

// Inherits New() from the Store.
var _ rest.Storage = &REST{}

// Inherits a lot of methods from the Store.  A &REST also implements
// Exporter, Scoper, TableConvertor, Creater, Updater by inheritance
// from the Store.
var _ rest.StandardStorage = &REST{}

// Implement ShortNamesProvider
var _ rest.ShortNamesProvider = &REST{}

// ShortNames implements the ShortNamesProvider interface. Returns a list of short names for a resource.
func (r *REST) ShortNames() []string {
	return r.ShortNamez
}

// Implement CategoriesProvider
var _ rest.CategoriesProvider = &REST{}

// Categories implements the CategoriesProvider interface. Returns a list of categories a resource is part of.
func (r *REST) Categories() []string {
	return r.Categorys
}

func (r *REST) WithCategories(categories []string) *REST {
	r.Categorys = categories
	return r
}

// StatusREST implements the REST endpoint for changing the status of an object
type StatusREST struct {
	Stor *genericregistry.Store
}

var _ rest.Storage = &StatusREST{}
var _ rest.Getter = &StatusREST{}
var _ rest.Updater = &StatusREST{}

func (r *StatusREST) New() runtime.Object {
	return r.Stor.New()
}

// Get retrieves the object from the storage. It is required to support Patch.
func (r *StatusREST) Get(ctx context.Context, name string, options *metav1.GetOptions) (runtime.Object, error) {
	return r.Stor.Get(ctx, name, options)
}

// Update alters the status subset of an object.
func (r *StatusREST) Update(ctx context.Context, name string, objInfo rest.UpdatedObjectInfo, createValidation rest.ValidateObjectFunc, updateValidation rest.ValidateObjectUpdateFunc, forceAllowCreate bool, options *metav1.UpdateOptions) (runtime.Object, bool, error) {
	// We are explicitly setting forceAllowCreate to false in the call to the underlying storage because
	// subresources should never allow create on update.
	return r.Stor.Update(ctx, name, objInfo, createValidation, updateValidation, false, options)
}
