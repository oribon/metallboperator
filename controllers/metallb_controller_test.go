package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	"github.com/metallb/metallb-operator/test/consts"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("MetalLB Controller", func() {
	Context("syncMetalLB", func() {

		AfterEach(func() {
			err := cleanTestNamespace()
			Expect(err).ToNot(HaveOccurred())
		})

		It("Should create manifests with images and namespace overriden", func() {

			metallb := &metallbv1beta1.MetalLB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metallb",
					Namespace: MetalLBTestNameSpace,
				},
			}

			speakerImage := "test-speaker:latest"
			controllerImage := "test-controller:latest"
			frrImage := "test-frr:latest"
			kubeRbacImage := "test-kube-rbac-proxy:latest"

			controllerContainers := map[string]string{
				"controller":      controllerImage,
				"kube-rbac-proxy": kubeRbacImage,
			}

			speakerContainers := map[string]string{
				"speaker":             speakerImage,
				"frr":                 frrImage,
				"reloader":            frrImage,
				"frr-metrics":         frrImage,
				"kube-rbac-proxy":     kubeRbacImage,
				"kube-rbac-proxy-frr": kubeRbacImage,
			}

			speakerInitContainers := map[string]string{
				"cp-frr-files": frrImage,
				"cp-reloader":  speakerImage,
				"cp-metrics":   speakerImage,
				"cp-liveness":  speakerImage,
			}

			By("Creating a MetalLB resource")
			err := k8sClient.Create(context.Background(), metallb)
			Expect(err).ToNot(HaveOccurred())

			By("Validating that the variables were templated correctly")
			controllerDeployment := &appsv1.Deployment{}
			Eventually(func() error {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: consts.MetalLBDeploymentName, Namespace: MetalLBTestNameSpace}, controllerDeployment)
				return err
			}, 2*time.Second, 200*time.Millisecond).ShouldNot((HaveOccurred()))
			Expect(controllerDeployment).NotTo(BeZero())
			Expect(len(controllerDeployment.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
			for _, c := range controllerDeployment.Spec.Template.Spec.Containers {
				image, ok := controllerContainers[c.Name]
				Expect(ok).To(BeTrue(), fmt.Sprintf("container %s not found in %s", c.Name, controllerContainers))
				Expect(c.Image).To(Equal(image))
			}

			speakerDaemonSet := &appsv1.DaemonSet{}
			Eventually(func() error {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: consts.MetalLBDaemonsetName, Namespace: MetalLBTestNameSpace}, speakerDaemonSet)
				return err
			}, 2*time.Second, 200*time.Millisecond).ShouldNot((HaveOccurred()))
			Expect(speakerDaemonSet).NotTo(BeZero())
			Expect(len(speakerDaemonSet.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
			for _, c := range speakerDaemonSet.Spec.Template.Spec.Containers {
				image, ok := speakerContainers[c.Name]
				Expect(ok).To(BeTrue(), fmt.Sprintf("container %s not found in %s", c.Name, speakerContainers))
				Expect(c.Image).To(Equal(image))
			}
			for _, c := range speakerDaemonSet.Spec.Template.Spec.InitContainers {
				image, ok := speakerInitContainers[c.Name]
				Expect(ok).To(BeTrue(), fmt.Sprintf("init container %s not found in %s", c.Name, speakerInitContainers))
				Expect(c.Image).To(Equal(image))
			}

			metallb = &metallbv1beta1.MetalLB{}
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "metallb", Namespace: MetalLBTestNameSpace}, metallb)
				Expect(err).NotTo(HaveOccurred())
				By("Specify the SpeakerNodeSelector")
				metallb.Spec.SpeakerNodeSelector = map[string]string{"kubernetes.io/os": "linux", "node-role.kubernetes.io/worker": "true"}
				return k8sClient.Update(context.TODO(), metallb)
			})
			Expect(err).NotTo(HaveOccurred())
			speakerDaemonSet = &appsv1.DaemonSet{}
			Eventually(func() map[string]string {
				err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: consts.MetalLBDaemonsetName, Namespace: MetalLBTestNameSpace}, speakerDaemonSet)
				if err != nil {
					return nil
				}
				return speakerDaemonSet.Spec.Template.Spec.NodeSelector
			}, 2*time.Second, 200*time.Millisecond).Should(Equal(metallb.Spec.SpeakerNodeSelector))
			Expect(speakerDaemonSet).NotTo(BeZero())
			Expect(len(speakerDaemonSet.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
			// Reset nodeSelector configuration
			metallb = &metallbv1beta1.MetalLB{}
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "metallb", Namespace: MetalLBTestNameSpace}, metallb)
				Expect(err).NotTo(HaveOccurred())
				metallb.Spec.SpeakerNodeSelector = map[string]string{}
				return k8sClient.Update(context.TODO(), metallb)
			})
			Expect(err).NotTo(HaveOccurred())

			metallb = &metallbv1beta1.MetalLB{}
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "metallb", Namespace: MetalLBTestNameSpace}, metallb)
				Expect(err).NotTo(HaveOccurred())
				By("Specify the speaker's Tolerations")
				metallb.Spec.SpeakerTolerations = []v1.Toleration{
					{
						Key:      "example1",
						Operator: v1.TolerationOpExists,
						Effect:   v1.TaintEffectNoExecute,
					},
					{
						Key:      "example2",
						Operator: v1.TolerationOpExists,
						Effect:   v1.TaintEffectNoExecute,
					},
				}
				return k8sClient.Update(context.TODO(), metallb)
			})
			Expect(err).NotTo(HaveOccurred())

			metallb = &metallbv1beta1.MetalLB{}
			err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "metallb", Namespace: MetalLBTestNameSpace}, metallb)
			Expect(err).NotTo(HaveOccurred())
			speakerDaemonSet = &appsv1.DaemonSet{}
			expectedTolerations := []v1.Toleration{
				{
					Key:               "node-role.kubernetes.io/master",
					Operator:          "Exists",
					Value:             "",
					Effect:            "NoSchedule",
					TolerationSeconds: nil,
				},
				{
					Key:               "node-role.kubernetes.io/control-plane",
					Operator:          "Exists",
					Value:             "",
					Effect:            "NoSchedule",
					TolerationSeconds: nil,
				},
				{
					Key:      "example1",
					Operator: v1.TolerationOpExists,
					Effect:   v1.TaintEffectNoExecute,
				},
				{
					Key:      "example2",
					Operator: v1.TolerationOpExists,
					Effect:   v1.TaintEffectNoExecute,
				},
			}
			Eventually(func() []v1.Toleration {
				err := k8sClient.Get(context.TODO(), types.NamespacedName{Name: consts.MetalLBDaemonsetName, Namespace: MetalLBTestNameSpace}, speakerDaemonSet)
				if err != nil {
					return nil
				}
				return speakerDaemonSet.Spec.Template.Spec.Tolerations
			}, 2*time.Second, 200*time.Millisecond).Should(Equal(expectedTolerations))
			Expect(speakerDaemonSet).NotTo(BeZero())
			Expect(len(speakerDaemonSet.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
			// Reset toleration configuration
			metallb = &metallbv1beta1.MetalLB{}
			err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
				err = k8sClient.Get(context.Background(), types.NamespacedName{Name: "metallb", Namespace: MetalLBTestNameSpace}, metallb)
				Expect(err).NotTo(HaveOccurred())
				metallb.Spec.SpeakerTolerations = nil
				return k8sClient.Update(context.TODO(), metallb)
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should forward logLevel to containers", func() {

			metallb := &metallbv1beta1.MetalLB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metallb",
					Namespace: MetalLBTestNameSpace,
				},
				Spec: metallbv1beta1.MetalLBSpec{
					LogLevel: metallbv1beta1.LogLevelWarn,
				},
			}

			err := k8sClient.Create(context.Background(), metallb)
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() []v1.Container {
				speakerDaemonSet := &appsv1.DaemonSet{}
				err := k8sClient.Get(
					context.Background(),
					types.NamespacedName{Name: consts.MetalLBDaemonsetName, Namespace: MetalLBTestNameSpace},
					speakerDaemonSet)
				if err != nil {
					return nil
				}

				return speakerDaemonSet.Spec.Template.Spec.Containers
			}, 2*time.Second, 200*time.Millisecond).Should(
				ContainElement(
					And(
						WithTransform(nameGetter, Equal("speaker")),
						WithTransform(argsGetter, ContainElement("--log-level=warn")),
					)))

			controllerDeployment := &appsv1.Deployment{}
			Eventually(func() []v1.Container {
				err := k8sClient.Get(
					context.Background(),
					types.NamespacedName{Name: consts.MetalLBDeploymentName, Namespace: MetalLBTestNameSpace},
					controllerDeployment,
				)
				if err != nil {
					return nil
				}
				return controllerDeployment.Spec.Template.Spec.Containers
			}, 2*time.Second, 200*time.Millisecond).Should(
				ContainElement(
					And(
						WithTransform(nameGetter, Equal("controller")),
						WithTransform(argsGetter, ContainElement("--log-level=warn")),
					)))
		})

		It("Should create manifests with images and namespace overriden for frr-k8s", func() {
			if os.Getenv("METALLB_BGP_TYPE") != bgpFRRK8S {
				Skip("irrelevant for non frr-k8s deployments")
			}

			metallb := &metallbv1beta1.MetalLB{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "metallb",
					Namespace: MetalLBTestNameSpace,
				},
			}

			frrk8sImage := "test-frr-k8s:latest"
			frrImage := "test-frr:latest"
			kubeRbacImage := "test-kube-rbac-proxy:latest"

			frrk8sContainers := map[string]string{
				"controller":          frrk8sImage,
				"frr":                 frrImage,
				"reloader":            frrImage,
				"frr-metrics":         frrImage,
				"kube-rbac-proxy":     kubeRbacImage,
				"kube-rbac-proxy-frr": kubeRbacImage,
			}

			frrk8sInitContainers := map[string]string{
				"cp-frr-files": frrImage,
				"cp-reloader":  frrk8sImage,
				"cp-metrics":   frrk8sImage,
				"cp-liveness":  frrk8sImage,
			}

			By("Creating a MetalLB resource")
			err := k8sClient.Create(context.Background(), metallb)
			Expect(err).ToNot(HaveOccurred())

			By("Validating that the variables were templated correctly")
			frrk8sDaemonSet := &appsv1.DaemonSet{}
			Eventually(func() error {
				err := k8sClient.Get(context.Background(), types.NamespacedName{Name: consts.FRRK8SDaemonsetName, Namespace: MetalLBTestNameSpace}, frrk8sDaemonSet)
				return err
			}, 2*time.Second, 200*time.Millisecond).ShouldNot((HaveOccurred()))
			Expect(frrk8sDaemonSet).NotTo(BeZero())
			Expect(len(frrk8sDaemonSet.Spec.Template.Spec.Containers)).To(BeNumerically(">", 0))
			for _, c := range frrk8sDaemonSet.Spec.Template.Spec.Containers {
				image, ok := frrk8sContainers[c.Name]
				Expect(ok).To(BeTrue(), fmt.Sprintf("container %s not found in %s", c.Name, frrk8sContainers))
				Expect(c.Image).To(Equal(image))
			}
			for _, c := range frrk8sDaemonSet.Spec.Template.Spec.InitContainers {
				image, ok := frrk8sInitContainers[c.Name]
				Expect(ok).To(BeTrue(), fmt.Sprintf("init container %s not found in %s", c.Name, frrk8sInitContainers))
				Expect(c.Image).To(Equal(image))
			}
		})
	})
})

func cleanTestNamespace() error {
	err := k8sClient.DeleteAllOf(context.Background(), &metallbv1beta1.MetalLB{}, client.InNamespace(MetalLBTestNameSpace))
	if err != nil {
		return err
	}
	err = k8sClient.DeleteAllOf(context.Background(), &appsv1.Deployment{}, client.InNamespace(MetalLBTestNameSpace))
	if err != nil {
		return err
	}
	err = k8sClient.DeleteAllOf(context.Background(), &appsv1.DaemonSet{}, client.InNamespace(MetalLBTestNameSpace))
	return err
}

// Gomega transformation functions for v1.Container
func argsGetter(c v1.Container) []string { return c.Args }
func nameGetter(c v1.Container) string   { return c.Name }
