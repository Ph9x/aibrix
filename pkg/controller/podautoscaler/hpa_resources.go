/*
Copyright 2024.

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

package podautoscaler

import (
	"context"
	"fmt"
	pa_v1 "github.com/aibrix/aibrix/api/autoscaling/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
	"math"
	"strconv"
	"strings"
)

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
)

var (
	controllerKind = pa_v1.GroupVersion.WithKind("PodAutoScaler") // Define the resource type for the controller
)

func getHPANameFromPa(pa *pa_v1.PodAutoscaler) string {
	return fmt.Sprintf("%s-hpa", pa.Name)
}

// MakeHPA creates an HPA resource from a PodAutoscaler resource.
func MakeHPA(pa *pa_v1.PodAutoscaler, ctx context.Context) *autoscalingv2.HorizontalPodAutoscaler {
	minReplicas, maxReplicas := pa.Spec.MinReplicas, pa.Spec.MaxReplicas
	if maxReplicas == 0 {
		maxReplicas = math.MaxInt32 // Set default to no upper limit if not specified
	}
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getHPANameFromPa(pa),
			Namespace:   pa.Namespace,
			Labels:      pa.Labels,
			Annotations: pa.Annotations,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(pa.GetObjectMeta(), controllerKind),
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: pa.Spec.ScaleTargetRef.APIVersion,
				Kind:       pa.Spec.ScaleTargetRef.Kind,
				Name:       pa.Spec.ScaleTargetRef.Name,
			},
			MaxReplicas: maxReplicas,
		},
	}
	if minReplicas != nil && *minReplicas > 0 {
		hpa.Spec.MinReplicas = minReplicas
	}

	if targetValue, err := strconv.ParseFloat(pa.Spec.TargetValue, 64); err != nil {
		klog.V(3).ErrorS(err, "Failed to parse target value")
	} else {
		klog.V(3).InfoS("Creating HPA", "metric", pa.Spec.TargetMetric, "target", targetValue)

		switch strings.ToLower(pa.Spec.TargetMetric) {
		case pa_v1.CPU:
			utilValue := int32(math.Ceil(targetValue))
			hpa.Spec.Metrics = []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{
						Type:               autoscalingv2.UtilizationMetricType,
						AverageUtilization: &utilValue,
					},
				},
			}}

		case pa_v1.Memory:
			memory := resource.NewQuantity(int64(targetValue)*1024*1024, resource.BinarySI)
			hpa.Spec.Metrics = []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name: corev1.ResourceMemory,
					Target: autoscalingv2.MetricTarget{
						Type:         autoscalingv2.AverageValueMetricType,
						AverageValue: memory,
					},
				},
			}}

		default:
			targetQuantity := resource.NewQuantity(int64(targetValue), resource.DecimalSI)
			hpa.Spec.Metrics = []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.PodsMetricSourceType,
				Pods: &autoscalingv2.PodsMetricSource{
					Metric: autoscalingv2.MetricIdentifier{
						Name: pa.Spec.TargetMetric,
					},
					Target: autoscalingv2.MetricTarget{
						Type:         autoscalingv2.AverageValueMetricType,
						AverageValue: targetQuantity,
					},
				},
			}}
		}
	}

	return hpa
}
