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

// Package ovs defines a network fabric that implements k8s.io/examples/staging/kos/pkg/networkfabric.Interface .
//
// The fabric is thread-safe.
//
// The fabric is based on an Open vSwitch (OvS) bridge, which gets configured
// through OpenFlow.
//
//******************************************************************************
// Currently, THE FABRIC DOES NOT WORK IF ANOTHER OvS BRIDGE EXISTS ON THE NODE.
// DOES NOT WORK means that its behavior is incorrect and undefined. Until this
// limitation is removed, users must ensure there's no other OvS bridge on the
// node.
//******************************************************************************
//
// The fabric constructor's creates the OvS bridge and a Network Interface
// configured to act as a VXLAN Tunnel End Point (VTEP), and connects them
// together. It also installs some default OpenFlow flows in the bridge.
// All these operations are idempotent so that it is safe for the process that
// uses the fabric to restart after a failure and re-instantiate the fabric.
// This is important because the bridge and the VTEP outlive the process that
// created them.
//
// A local Network Interface is implemented as Linux network device connected
// to the bridge and three OpenFlow flows that allow the network device to send
// and receive traffic. One flow encapsulates all traffic coming from the Linux
// network device in a VXLAN packet, one flow forwards ARP requests for the
// Network Interface IP and VNI to the Linux network device, one flow forwards
// normal Layer 2 frames for the Network Interface MAC address and VNI to the
// Linux network device.
// The flows are added/removed atomically to/from the bridge, but the creation
// of the Linux network device and the addition of the flows are not executed
// atomically. If a failure occurs after the creation of the Linux network
// device but before (or during) the addition of the flows, a one-shot attempt
// to delete the Linux network device is done; if it fails the fabric gives up
// and an incomplete implementation of the Network Interface is left on the node.
//
// A remote Network Interface is implemented as two OpenFlow flows. One flow
// sends ARP requests for the remote Network Interface IP and VNI to the
// remote Network Interface host through the VTEP of the fabric's bridge. The
// other flow does the same thing, except for Layer 2 frames for the Network
// Interface MAC address and VNI instead of ARP requests. The two flows are
// added/removed atomically to/from the fabric's bridge.
//
// Currently, all operations on the bridge are done using the OvS CLI (through
// Golang's os.Exec).
//
// To be able to create an ovs network fabric in an application, you need to
// import package ovs in the main package of the application. This ensures that
// the factory which creates ovs network fabrics is registered in the network
// fabric factory registry, and can therefore be used to instantiate network
// fabrics.
package ovs // import "k8s.io/examples/staging/kos/pkg/networkfabric/ovs"
