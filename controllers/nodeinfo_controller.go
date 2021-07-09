/*
Copyright 2021.

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

package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	topologyv1 "vkubeviewer/api/v1"
)

// NodeInfoReconciler reconciles a NodeInfo object
type NodeInfoReconciler struct {
	client.Client
	VC     *vim25.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=topology.vkubeviewer.com,resources=nodeinfoes,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=topology.vkubeviewer.com,resources=nodeinfoes/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=topology.vkubeviewer.com,resources=nodeinfoes/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NodeInfo object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *NodeInfoReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	ctx = context.Background()
	log := r.Log.WithValues("NodeInfo", req.NamespacedName)

	ch := &topologyv1.NodeInfo{}
	if err := r.Client.Get(ctx, req.NamespacedName, ch); err != nil {
		// add some debug information if it's not a NotFound error
		if !k8serr.IsNotFound(err) {
			log.Error(err, "unable to fetch NodeInfo")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	msg := fmt.Sprintf("received reconcile request for %q (namespace: %q)", ch.GetName(), ch.GetNamespace())
	log.Info(msg)

	//
	// Create a view manager
	//

	m := view.NewManager(r.VC)

	//
	// Create a container view of VirtualMachine objects
	//

	v, err := m.CreateContainerView(ctx, r.VC.ServiceContent.RootFolder, []string{"VirtualMachine"}, true)

	if err != nil {
		msg := fmt.Sprintf("unable to create container view for VirtualMachines: error %s", err)
		log.Info(msg)
		return ctrl.Result{}, err
	}

	defer v.Destroy(ctx)

	//
	// Retrieve summary property for all VMs
	//

	var vms []mo.VirtualMachine

	err = v.Retrieve(ctx, []string{"VirtualMachine"}, []string{"summary"}, &vms)

	if err != nil {
		msg := fmt.Sprintf("unable to retrieve VM summary: error %s", err)
		log.Info(msg)
		return ctrl.Result{}, err
	}

	//
	// Print summary for host in NodeInfo specification info
	//

	for _, vm := range vms {
		if vm.Summary.Config.Name == ch.Spec.Nodename {
			ch.Status.GuestId = string(vm.Summary.Guest.GuestId)
			ch.Status.TotalCPU = int64(vm.Summary.Config.NumCpu)
			ch.Status.ResvdCPU = int64(vm.Summary.Config.CpuReservation)
			ch.Status.TotalMem = int64(vm.Summary.Config.MemorySizeMB)
			ch.Status.ResvdMem = int64(vm.Summary.Config.MemoryReservation)
			ch.Status.PowerState = string(vm.Summary.Runtime.PowerState)
			ch.Status.HwVersion = string(vm.Summary.Guest.HwVersion)
			ch.Status.IpAddress = string(vm.Summary.Guest.IpAddress)
			ch.Status.PathToVM = string(vm.Summary.Config.VmPathName)
		}
	}

	if err := r.Status().Update(ctx, ch); err != nil {
		log.Error(err, "unable to update NodeInfo status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: time.Duration(1) * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NodeInfoReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&topologyv1.NodeInfo{}).
		Complete(r)
}
