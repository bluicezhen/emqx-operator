package v2alpha2

import (
	"context"
	"fmt"

	appsv2alpha2 "github.com/emqx/emqx-operator/apis/apps/v2alpha2"
	controllerv2alpha2 "github.com/emqx/emqx-operator/controllers/apps/v2alpha2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8sErrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var rebalance = appsv2alpha2.Rebalance{
	ObjectMeta: metav1.ObjectMeta{
		Name: "rebalance",
	},
	Spec: appsv2alpha2.RebalanceSpec{
		RebalanceStrategy: appsv2alpha2.RebalanceStrategy{
			WaitTakeover:     10,
			ConnEvictRate:    10,
			SessEvictRate:    10,
			WaitHealthCheck:  10,
			AbsSessThreshold: 100,
			RelConnThreshold: "1.2",
			AbsConnThreshold: 100,
			RelSessThreshold: "1.2",
		},
	},
}

var _ = Describe("Emqx v2alpha2 Rebalance Test", Label("rebalance"), func() {
	var instance *appsv2alpha2.EMQX
	var r *appsv2alpha2.Rebalance
	BeforeEach(func() {
		instance = emqx.DeepCopy()
		instance.Spec.Image = "emqx/emqx-enterprise:5.1.1-alpha.2"
		instance.Default()
	})

	Context("EMQX is not found", func() {
		BeforeEach(func() {
			r = rebalance.DeepCopy()
			r.Namespace = instance.GetNamespace() + "-" + rand.String(5)
			r.Spec.InstanceName = "fake"
			r.Spec.InstanceKind = controllerv2alpha2.V2alpha2InstanceKind

			By("Creating namespace", func() {
				Expect(k8sClient.Create(context.TODO(), &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: r.Namespace,
						Labels: map[string]string{
							"test": "e2e",
						},
					},
				})).Should(Succeed())
			})

			By("Creating Rebalance CR", func() {
				Expect(k8sClient.Create(context.TODO(), r)).Should(Succeed())
			})
		})

		AfterEach(func() {
			By("Deleting Rebalance CR, can be successfull", func() {
				Eventually(func() error {
					return k8sClient.Delete(context.TODO(), r)
				}, timeout, interval).Should(Succeed())
				Eventually(func() bool {
					return k8sErrors.IsNotFound(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(r), r))
				}).Should(BeTrue())
			})
		})

		It("Rebalance will failed, because the EMQX is not found", func() {
			Eventually(func() appsv2alpha2.RebalanceStatus {
				_ = k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(r), r)
				return r.Status
			}, timeout, interval).Should(And(
				HaveField("Phase", appsv2alpha2.RebalancePhaseFailed),
				HaveField("RebalanceStates", BeNil()),
				HaveField("Conditions", ContainElements(
					HaveField("Type", appsv2alpha2.RebalanceConditionFailed),
				)),
				HaveField("Conditions", ContainElements(
					HaveField("Status", corev1.ConditionTrue),
				)),
				HaveField("Conditions", ContainElements(
					HaveField("Message", fmt.Sprintf("EMQX %s is not found", r.Spec.InstanceName)),
				)),
			))
		})
	})

	Context("Enterprise is found", func() {
		BeforeEach(func() {
			r = rebalance.DeepCopy()

			By("Creating namespace", func() {
				// create namespace
				Expect(k8sClient.Create(context.TODO(), &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: instance.GetNamespace(),
						Labels: map[string]string{
							"test": "e2e",
						},
					},
				})).Should(Succeed())
			})

			By("Creating EMQX CR", func() {
				// create EMQX CR
				instance.Spec.ReplicantTemplate = nil
				instance.Spec.CoreTemplate.Spec.Replicas = pointer.Int32Ptr(2)
				instance.Default()
				Expect(instance.ValidateCreate()).Should(Succeed())
				Expect(k8sClient.Create(context.TODO(), instance)).Should(Succeed())
			})

			By("Checking EMQX CR", func() {
				// check EMQX CR if created successfully
				Eventually(func() *appsv2alpha2.EMQX {
					_ = k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(instance), instance)
					return instance
				}).WithTimeout(timeout).WithPolling(interval).Should(
					And(
						WithTransform(func(instance *appsv2alpha2.EMQX) bool {
							return instance.Status.IsConditionTrue(appsv2alpha2.Ready)
						}, BeTrue()),
						WithTransform(func(instance *appsv2alpha2.EMQX) []appsv2alpha2.EMQXNode {
							return instance.Status.CoreNodes
						}, HaveLen(int(*instance.Spec.CoreTemplate.Spec.Replicas))),
						WithTransform(func(instance *appsv2alpha2.EMQX) appsv2alpha2.EMQXNodesStatus {
							return instance.Status.CoreNodesStatus
						}, And(
							HaveField("Replicas", Equal(int32(*instance.Spec.CoreTemplate.Spec.Replicas))),
							HaveField("ReadyReplicas", Equal(int32(*instance.Spec.CoreTemplate.Spec.Replicas))),
							HaveField("CurrentRevision", Not(BeEmpty())),
							HaveField("CurrentReplicas", Equal(int32(*instance.Spec.CoreTemplate.Spec.Replicas))),
							HaveField("UpdateRevision", Not(BeEmpty())),
							HaveField("UpdateReplicas", Equal(int32(*instance.Spec.CoreTemplate.Spec.Replicas))),
						)),
						WithTransform(func(instance *appsv2alpha2.EMQX) []appsv2alpha2.EMQXNode {
							return instance.Status.ReplicantNodes
						}, BeNil()),
						WithTransform(func(instance *appsv2alpha2.EMQX) *appsv2alpha2.EMQXNodesStatus {
							return instance.Status.ReplicantNodesStatus
						}, BeNil()),
					),
				)
			})
		})

		AfterEach(func() {
			By("Deleting EMQX CR", func() {
				// delete emqx cr
				Eventually(func() error {
					return k8sClient.Delete(context.TODO(), instance)
				}, timeout, interval).Should(Succeed())
				Eventually(func() bool {
					return k8sErrors.IsNotFound(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(instance), instance))
				}).Should(BeTrue())
			})

			// delete rebalance cr
			By("Deleting Rebalance CR", func() {
				Eventually(func() error {
					return k8sClient.Delete(context.TODO(), r)
				}, timeout, interval).Should(Succeed())
				Eventually(func() bool {
					return k8sErrors.IsNotFound(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(r), r))
				}).Should(BeTrue())
			})
		})

		It("Check rebalance status", func() {
			By("Create rebalance", func() {
				r.Namespace = instance.GetNamespace()
				r.Spec.InstanceName = instance.GetName()
				r.Spec.InstanceKind = controllerv2alpha2.V2alpha2InstanceKind
				Expect(k8sClient.Create(context.TODO(), r)).Should(Succeed())
			})

			By("Rebalance should have finalizer", func() {
				Eventually(func() []string {
					_ = k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(r), r)
					return r.GetFinalizers()
				}, timeout, interval).Should(ContainElements("apps.emqx.io/finalizer"))
			})

			By("Rebalance will failed, because the EMQX Enterprise is nothing to balance", func() {
				Eventually(func() appsv2alpha2.RebalanceStatus {
					_ = k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(r), r)
					return r.Status
				}, timeout, interval).Should(And(
					HaveField("Phase", appsv2alpha2.RebalancePhaseFailed),
					HaveField("RebalanceStates", BeNil()),
					HaveField("Conditions", ContainElements(
						And(
							HaveField("Type", appsv2alpha2.RebalanceConditionFailed),
							HaveField("Status", corev1.ConditionTrue),
							HaveField("Message", "Failed to start rebalance: request api failed: 400 Bad Request"),
						),
					)),
				))
			})

			By("Mock rebalance is in progress", func() {
				// mock rebalance processing
				r.Status.Phase = appsv2alpha2.RebalancePhaseProcessing
				r.Status.Conditions = []appsv2alpha2.RebalanceCondition{}
				Expect(k8sClient.Status().Update(context.TODO(), r)).Should(Succeed())

				// update annotations for target reconciler
				Expect(k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(r), r)).Should(Succeed())
				r.Annotations = map[string]string{"test": "e2e"}
				Expect(k8sClient.Update(context.TODO(), r)).Should(Succeed())
			})

			By("Rebalance should completed", func() {
				Eventually(func() appsv2alpha2.RebalanceStatus {
					_ = k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(r), r)
					return r.Status
				}, timeout, interval).Should(And(
					HaveField("Phase", appsv2alpha2.RebalancePhaseCompleted),
					HaveField("RebalanceStates", BeNil()),
					HaveField("Conditions", ContainElements(
						HaveField("Type", appsv2alpha2.RebalanceConditionCompleted),
					)),
					HaveField("Conditions", ContainElements(
						HaveField("Status", corev1.ConditionTrue),
					)),
				))
			})

		})
	})
})
