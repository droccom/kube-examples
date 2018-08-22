/*
Copyright 2018 The Kubernetes Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
	v1alpha1 "k8s.io/examples/staging/kos/pkg/apis/network/v1alpha1"
)

// IPLockLister helps list IPLocks.
type IPLockLister interface {
	// List lists all IPLocks in the indexer.
	List(selector labels.Selector) (ret []*v1alpha1.IPLock, err error)
	// IPLocks returns an object that can list and get IPLocks.
	IPLocks(namespace string) IPLockNamespaceLister
	IPLockListerExpansion
}

// iPLockLister implements the IPLockLister interface.
type iPLockLister struct {
	indexer cache.Indexer
}

// NewIPLockLister returns a new IPLockLister.
func NewIPLockLister(indexer cache.Indexer) IPLockLister {
	return &iPLockLister{indexer: indexer}
}

// List lists all IPLocks in the indexer.
func (s *iPLockLister) List(selector labels.Selector) (ret []*v1alpha1.IPLock, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.IPLock))
	})
	return ret, err
}

// IPLocks returns an object that can list and get IPLocks.
func (s *iPLockLister) IPLocks(namespace string) IPLockNamespaceLister {
	return iPLockNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// IPLockNamespaceLister helps list and get IPLocks.
type IPLockNamespaceLister interface {
	// List lists all IPLocks in the indexer for a given namespace.
	List(selector labels.Selector) (ret []*v1alpha1.IPLock, err error)
	// Get retrieves the IPLock from the indexer for a given namespace and name.
	Get(name string) (*v1alpha1.IPLock, error)
	IPLockNamespaceListerExpansion
}

// iPLockNamespaceLister implements the IPLockNamespaceLister
// interface.
type iPLockNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all IPLocks in the indexer for a given namespace.
func (s iPLockNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.IPLock, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.IPLock))
	})
	return ret, err
}

// Get retrieves the IPLock from the indexer for a given namespace and name.
func (s iPLockNamespaceLister) Get(name string) (*v1alpha1.IPLock, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("iplock"), name)
	}
	return obj.(*v1alpha1.IPLock), nil
}
