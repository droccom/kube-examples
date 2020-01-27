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

package networkfabric

import "net"

// Interface is the contract of a VXLAN network fabric.
// It declares functions to create Network Interfaces that are part of a VXLAN
// segment.
// The VXLAN segment of a Network Interface is an invocation argument of the
// functions to create the Network Interface.
// All traffic sent/received to/from the Network Interfaces created via an
// implementer of this contract MUST be VXLAN-tunneled.
//
// This contract makes a distinction between local and remote Network Interfaces.
//
// A local Network Interface is the networking state of a guest that is bound to
// the same node as the user of this contract. Creating a local Network
// Interface means creating a Linux network device and configuring the node's
// networking state so that the Linux network device can send/receive VXLAN-tunneled
// traffic to/from other guests (local or remote) in its VXLAN segment.
//
// A remote Network Interface is the networking state of a guest that is bound
// to a node other than that of the user of this contract. Creating a remote
// Network Interface means configuring the networking state on the node of the
// user of this contract so that traffic generated on such node and directed at
// the remote Network Interface is correctly VXLAN-tunneled to the node of the
// remote Network Interface.
//
// Network Interfaces are identified by a name, which must be unique over space.
// (VNI, guest IP) pairs must also be unique over space, that is, for this
// contract two Network Interfaces with the same (VNI, guest IP) pair are the
// same Network Interface. Follow some guarantees that implementers MUST make:
//
// (1) After a Network Interface X is created, fabric calls to create a Network
//     Interface Y with the same (VNI, guest IP) pair as X fail until the fabric
//     is used to delete X, regardless of the relationship between X and Y's
//     other fields, and regardless of whether X and Y are local or remote.
//
// (2) Two concurrent calls to create two Network Interfaces with the same
//     (VNI, guest IP) pair cannot both succeed: one will fail, one will succeed.
//     This is true regardless of the relationship between X and Y's other (than
//     guest IP and VNI) fields and regardless of whether X and Y are local or
//     remote.
type Interface interface {
	// Name returns the name of the fabric.
	Name() string

	// CreateLocalIfc creates a local Network Interface described by `ifc`.
	CreateLocalIfc(ifc LocalNetIfc) error

	// DeleteLocalIfc deletes the local Network Interface described by `ifc`,
	// if it exists.
	DeleteLocalIfc(ifc LocalNetIfc) error

	// CreateRemoteIfc creates a remote Network Interface described by `ifc`.
	CreateRemoteIfc(ifc RemoteNetIfc) error

	// DeleteRemoteIfc deletes the remote Network Interface described by `ifc`,
	// if it exists.
	DeleteRemoteIfc(ifc RemoteNetIfc) error

	// ListLocalIfcs returns all the local Network Interfaces that exist on the
	// caller's node.
	// If a call to `CreateLocalIfc` successfully creates an interface X,
	// subsequent calls to `ListLocalIfcs` will include X in the results until
	// X is deleted with a call to `DeleteLocalIfc` or the hard state that was
	// created for X is deleted by another process; this property holds true
	// regardless of whether the process that invokes `ListLocalIfcs` is the
	// same process that created X and regardless of whether the process that
	// created X is still running.
	// ListLocalIfcs should always be called by users of a fabric BEFORE doing
	// any other operation through the fabric, to know which local Network
	// Interfaces were previously created on the node.
	// Creating local Network Interfaces might entail non-atomic operations, and
	// the process doing the creation might fail in the middle of it, leaving an
	// half-implemented Network Interface on the node. Implementers of this
	// contract can put clean-up actions to remove half-implemented Network
	// Interfaces in this function.
	ListLocalIfcs() ([]LocalNetIfc, error)

	// ListRemoteIfcs returns all the remote Network Interfaces that exist on
	// the caller's node.
	// If a call to `CreateRemoteIfc` successfully creates an interface X,
	// subsequent calls to `ListRemoteIfcs` will include X in the results until
	// X is deleted with a call to `DeleteRemoteIfc` or the hard state that was
	// created for X is deleted by another process; this property holds true
	// regardless of whether the process that invokes `ListRemoteIfcs` is the
	// same process that created X and regardless of whether the process that
	// created X is still running.
	// ListRemoteIfcs should always be called by users of a fabric BEFORE doing
	// any other operation through the fabric, to know which remote Network
	// Interfaces were previously created on the node.
	// Creating remote Network Interfaces might entail non-atomic operations, and
	// the process doing the creation might fail in the middle of it, leaving an
	// half-implemented Network Interface on the node. Implementers of this
	// contract can put clean-up actions to remove half-implemented Network
	// Interfaces in this function.
	ListRemoteIfcs() ([]RemoteNetIfc, error)
}

// LocalNetIfc describes a local Network Interface. It contains everything
// Interface.CreateLocalIfc needs to create a Linux network device and configure
// networking state so that the Linux network device can send/receive
// VXLAN-tunneled traffic.
type LocalNetIfc struct {
	Name     string
	VNI      uint32
	GuestMAC net.HardwareAddr
	GuestIP  net.IP
}

// RemoteNetIfc describes a remote Network Interface. It contains everything
// Interface.CreateRemoteIfc needs to configure networking state so that local
// Network Interfaces can send VXLAN-tunneled traffic to the remote Network
// Interface.
type RemoteNetIfc struct {
	VNI      uint32
	GuestMAC net.HardwareAddr
	GuestIP  net.IP
	HostIP   net.IP
}
