/*
Copyright 2020 The Kubernetes Authors.

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

package scope

import (
	"context"

	"github.com/pkg/errors"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	rosacontrolplanev1 "sigs.k8s.io/cluster-api-provider-aws/v2/controlplane/rosa/api/v1beta2"
	expinfrav1 "sigs.k8s.io/cluster-api-provider-aws/v2/exp/api/v1beta2"
	"sigs.k8s.io/cluster-api-provider-aws/v2/pkg/cloud"
	"sigs.k8s.io/cluster-api-provider-aws/v2/pkg/logger"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	expclusterv1 "sigs.k8s.io/cluster-api/exp/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
)

// RosaMachinePoolScopeParams defines the input parameters used to create a new Scope.
type RosaMachinePoolScopeParams struct {
	Client          client.Client
	Logger          *logger.Logger
	Cluster         *clusterv1.Cluster
	ControlPlane    *rosacontrolplanev1.ROSAControlPlane
	RosaMachinePool *expinfrav1.ROSAMachinePool
	MachinePool     *expclusterv1.MachinePool
	ControllerName  string
}

// NewRosaMachinePoolScope creates a new Scope from the supplied parameters.
// This is meant to be called for each reconcile iteration.
func NewRosaMachinePoolScope(params RosaMachinePoolScopeParams) (*RosaMachinePoolScope, error) {
	if params.ControlPlane == nil {
		return nil, errors.New("failed to generate new scope from nil RosaControlPlane")
	}
	if params.MachinePool == nil {
		return nil, errors.New("failed to generate new scope from nil MachinePool")
	}
	if params.RosaMachinePool == nil {
		return nil, errors.New("failed to generate new scope from nil RosaMachinePool")
	}
	if params.Logger == nil {
		log := klog.Background()
		params.Logger = logger.NewLogger(log)
	}

	ammpHelper, err := patch.NewHelper(params.RosaMachinePool, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init RosaMachinePool patch helper")
	}
	mpHelper, err := patch.NewHelper(params.MachinePool, params.Client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to init MachinePool patch helper")
	}

	return &RosaMachinePoolScope{
		Logger:                     *params.Logger,
		Client:                     params.Client,
		patchHelper:                ammpHelper,
		capiMachinePoolPatchHelper: mpHelper,

		Cluster:         params.Cluster,
		ControlPlane:    params.ControlPlane,
		RosaMachinePool: params.RosaMachinePool,
		MachinePool:     params.MachinePool,
		controllerName:  params.ControllerName,
	}, nil
}

// RosaMachinePoolScope defines the basic context for an actuator to operate upon.
type RosaMachinePoolScope struct {
	logger.Logger
	client.Client
	patchHelper                *patch.Helper
	capiMachinePoolPatchHelper *patch.Helper

	Cluster         *clusterv1.Cluster
	ControlPlane    *rosacontrolplanev1.ROSAControlPlane
	RosaMachinePool *expinfrav1.ROSAMachinePool
	MachinePool     *expclusterv1.MachinePool

	controllerName string
}

// RosaMachinePoolName returns the rosa machine pool name.
func (s *RosaMachinePoolScope) RosaMachinePoolName() string {
	return s.RosaMachinePool.Name
}

// NodePoolName returns the nodePool name of this machine pool.
func (s *RosaMachinePoolScope) NodePoolName() string {
	return s.RosaMachinePool.Spec.NodePoolName
}

// ClusterName returns the cluster name.
func (s *RosaMachinePoolScope) RosaClusterName() string {
	return s.ControlPlane.Spec.RosaClusterName
}

// ControlPlaneSubnets returns the control plane subnets.
func (s *RosaMachinePoolScope) ControlPlaneSubnets() []string {
	return s.ControlPlane.Spec.Subnets
}

// InfraCluster returns the AWS infrastructure cluster or control plane object.
func (s *RosaMachinePoolScope) InfraCluster() cloud.ClusterObject {
	return s.ControlPlane
}

// ClusterObj returns the cluster object.
func (s *RosaMachinePoolScope) ClusterObj() cloud.ClusterObject {
	return s.Cluster
}

// ControllerName returns the name of the controller that
// created the RosaMachinePool.
func (s *RosaMachinePoolScope) ControllerName() string {
	return s.controllerName
}

func (s *RosaMachinePoolScope) GetSetter() conditions.Setter {
	return s.RosaMachinePool
}

// RosaMchinePoolReadyFalse marks the ready condition false using warning if error isn't
// empty.
func (s *RosaMachinePoolScope) RosaMchinePoolReadyFalse(reason string, err string) error {
	severity := clusterv1.ConditionSeverityWarning
	if err == "" {
		severity = clusterv1.ConditionSeverityInfo
	}
	conditions.MarkFalse(
		s.RosaMachinePool,
		expinfrav1.RosaMachinePoolReadyCondition,
		reason,
		severity,
		err,
	)
	if err := s.PatchObject(); err != nil {
		return errors.Wrap(err, "failed to mark rosa machinepool not ready")
	}
	return nil
}

// PatchObject persists the control plane configuration and status.
func (s *RosaMachinePoolScope) PatchObject() error {
	return s.patchHelper.Patch(
		context.TODO(),
		s.RosaMachinePool,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			expinfrav1.RosaMachinePoolReadyCondition,
		}})
}

// PatchCAPIMachinePoolObject persists the capi machinepool configuration and status.
func (s *RosaMachinePoolScope) PatchCAPIMachinePoolObject(ctx context.Context) error {
	return s.capiMachinePoolPatchHelper.Patch(
		ctx,
		s.MachinePool,
	)
}

// Close closes the current scope persisting the control plane configuration and status.
func (s *RosaMachinePoolScope) Close() error {
	return s.PatchObject()
}
