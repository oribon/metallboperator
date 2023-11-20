package helm

import (
	"testing"

	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	invalidFRRK8SHelmChartPath  = "../../bindata/deployment/no-helm"
	frrk8sHelmChartPath         = "../../bindata/deployment/helm/frr-k8s"
	frrk8sHelmChartName         = "frr-k8s"
	FRRK8STestNameSpace         = "frr-k8s-test-namespace"
	frrk8sDaemonSetName         = "frr-k8s"
	frrk8sWebhookDeploymentName = "webhook-server"
)

func TestLoadFRRK8SChart(t *testing.T) {
	resetEnv()
	g := NewGomegaWithT(t)
	setEnv()
	_, err := InitFRRK8SChart(invalidFRRK8SHelmChartPath, frrk8sHelmChartName, MetalLBTestNameSpace, false)
	g.Expect(err).NotTo(BeNil())
	chart, err := InitFRRK8SChart(frrk8sHelmChartPath, frrk8sHelmChartName, MetalLBTestNameSpace, false)
	g.Expect(err).To(BeNil())
	g.Expect(chart.chart).NotTo(BeNil())
	g.Expect(chart.chart.Name()).To(Equal(frrk8sHelmChartName))
}

func TestParseFRRK8SChartWithCustomValues(t *testing.T) {
	resetEnv()

	g := NewGomegaWithT(t)
	setEnv(envVar{"FRRK8S_IMAGE", "frr-k8s:test"})
	chart, err := InitFRRK8SChart(frrk8sHelmChartPath, frrk8sHelmChartName, MetalLBTestNameSpace, false)
	g.Expect(err).To(BeNil())
	metallb := &metallbv1beta1.MetalLB{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "metallb",
			Namespace: MetalLBTestNameSpace,
		},
		Spec: metallbv1beta1.MetalLBSpec{
			LogLevel: metallbv1beta1.LogLevelDebug,
		},
	}

	objs, err := chart.GetObjects(metallb, false)
	g.Expect(err).To(BeNil())
	var isFRRK8SFound, isFRRK8SWebhookFound bool
	for _, obj := range objs {
		objKind := obj.GetKind()
		objName := obj.GetName()
		if objKind == "DaemonSet" && objName == frrk8sDaemonSetName {
			frrk8s := appsv1.DaemonSet{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &frrk8s)
			g.Expect(err).To(BeNil())
			g.Expect(frrk8s.GetName()).To(Equal(frrk8sDaemonSetName))
			var frrk8sControllerFound bool
			for _, container := range frrk8s.Spec.Template.Spec.Containers {
				if container.Name == "controller" {
					g.Expect(container.Image == "frr-k8s:test")
					frrk8sControllerFound = true

					logLevelChanged := false
					for _, a := range container.Args {
						if a == "--log-level=debug" {
							logLevelChanged = true
						}
					}
					g.Expect(logLevelChanged).To(BeTrue())
				}
			}
			g.Expect(frrk8sControllerFound).To(BeTrue())
			isFRRK8SFound = true
		}
		if objKind == "Deployment" && objName == frrk8sWebhookDeploymentName {
			frrk8sWebhook := appsv1.Deployment{}
			err = runtime.DefaultUnstructuredConverter.FromUnstructured(obj.UnstructuredContent(), &frrk8sWebhook)
			g.Expect(err).To(BeNil())
			g.Expect(frrk8sWebhook.GetName()).To(Equal(frrk8sWebhookDeploymentName))
			var webhookContainerFound bool
			for _, container := range frrk8sWebhook.Spec.Template.Spec.Containers {
				if container.Name == "frr-k8s-webhook-server" {
					g.Expect(container.Image == "frr-k8s:test")
					webhookContainerFound = true
				}
			}
			g.Expect(webhookContainerFound).To(BeTrue())
			isFRRK8SWebhookFound = true
		}
	}
	g.Expect(isFRRK8SFound).To(BeTrue())
	g.Expect(isFRRK8SWebhookFound).To(BeTrue())
}

func TestParseFRRK8SOCPSecureMetrics(t *testing.T) {
	resetEnv()
	setEnv(
		envVar{"DEPLOY_SERVICEMONITORS", "true"},
		envVar{"FRRK8S_HTTPS_METRICS_PORT", "9998"},
		envVar{"FRRK8S_FRR_HTTPS_METRICS_PORT", "9999"},
		envVar{"METALLB_BGP_TYPE", "frr-k8s"},
	)
	g := NewGomegaWithT(t)

	chart, err := InitFRRK8SChart(frrk8sHelmChartPath, frrk8sHelmChartName, MetalLBTestNameSpace, false)
	g.Expect(err).To(BeNil())
	metallb := &metallbv1beta1.MetalLB{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "metallb",
			Namespace: MetalLBTestNameSpace,
		},
	}

	objs, err := chart.GetObjects(metallb, true)
	g.Expect(err).To(BeNil())
	for _, obj := range objs {
		objKind := obj.GetKind()
		if objKind == "DaemonSet" {
			err = validateObject("ocp-metrics", "frr-k8s-daemon", obj)
			if err != nil {
				t.Fatalf("test ocp-metrics-frr-k8s-daemon failed. %s", err)
			}
		}
		if objKind == "ServiceMonitor" {
			err = validateObject("ocp-metrics", obj.GetName(), obj)
			if err != nil {
				t.Fatalf("test ocp-metrics-%s failed. %s", obj.GetName(), err)
			}
		}
		if objKind == "Deployment" {
			err = validateObject("ocp-metrics", "frr-k8s-webhook", obj)
			if err != nil {
				t.Fatalf("test ocp-metrics-frr-k8s-webhook failed. %s", err)
			}
		}
	}
}
