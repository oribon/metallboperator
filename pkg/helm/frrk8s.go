package helm

import (
	"fmt"
	"os"

	metallbv1beta1 "github.com/metallb/metallb-operator/api/v1beta1"
	"github.com/pkg/errors"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// FRRK8SChart contains references which helps to retrieve manifest
// from chart after patching given custom values.
type FRRK8SChart struct {
	client      *action.Install
	envSettings *cli.EnvSettings
	chart       *chart.Chart
	config      *frrK8SChartConfig
	namespace   string
}

type frrK8SChartConfig struct {
	namespace            string
	isOpenShift          bool
	frrk8sImage          *imageInfo
	frrImage             *imageInfo
	kubeRbacProxyImage   *imageInfo
	frrMetricsPort       int
	metricsPort          int
	secureMetricsPort    int
	secureFRRMetricsPort int
	enableServiceMonitor bool
}

// InitFRRK8SChart initializes frr-k8s helm chart after loading it from given
// chart path and creating config object from environment variables.
func InitFRRK8SChart(path, name, namespace string, isOpenshift bool) (*FRRK8SChart, error) {
	chart := &FRRK8SChart{}
	chart.namespace = namespace
	chart.envSettings = cli.New()
	chart.client = action.NewInstall(new(action.Configuration))
	chart.client.ReleaseName = name
	chart.client.DryRun = true
	chart.client.ClientOnly = true
	chart.client.Namespace = namespace
	chartPath, err := chart.client.ChartPathOptions.LocateChart(path, chart.envSettings)
	if err != nil {
		return nil, err
	}
	chart.chart, err = loader.Load(chartPath)
	if err != nil {
		return nil, err
	}
	chart.config, err = loadFRRK8SConfig(namespace, isOpenshift)
	if err != nil {
		return nil, err
	}
	return chart, nil
}

func loadFRRK8SConfig(namespace string, isOCP bool) (*frrK8SChartConfig, error) {
	config := &frrK8SChartConfig{
		isOpenShift:        isOCP,
		namespace:          namespace,
		kubeRbacProxyImage: &imageInfo{},
	}
	var err error
	frrk8sImage := os.Getenv("FRRK8S_IMAGE")
	if frrk8sImage != "" {
		controllerRepo, controllerTag := getImageNameTag(frrk8sImage)
		config.frrk8sImage = &imageInfo{controllerRepo, controllerTag}
	}

	frrImage := os.Getenv("FRR_IMAGE")
	if frrImage == "" {
		return nil, errors.Errorf("FRR_IMAGE env variable must be set for frr-k8s")
	}
	frrRepo, frrTag := getImageNameTag(frrImage)
	config.frrImage = &imageInfo{frrRepo, frrTag}

	config.metricsPort, err = valueWithDefault("FRRK8S_METRICS_PORT", 7572)
	if err != nil {
		return nil, err
	}
	config.secureMetricsPort, err = valueWithDefault("FRRK8S_HTTPS_METRICS_PORT", 9140)
	if err != nil {
		return nil, err
	}

	config.frrMetricsPort, err = valueWithDefault("FRRK8S_FRR_METRICS_PORT", 7573)
	if err != nil {
		return nil, err
	}
	config.secureFRRMetricsPort, err = valueWithDefault("FRRK8S_FRR_HTTPS_METRICS_PORT", 9141)
	if err != nil {
		return nil, err
	}

	// We shouldn't spam the api server trying to apply ServiceMonitors if the resource isn't installed.
	if os.Getenv("DEPLOY_SERVICEMONITORS") == "true" {
		config.enableServiceMonitor = true
	}

	kubeRbacProxyImage := os.Getenv("KUBE_RBAC_PROXY_IMAGE")
	if kubeRbacProxyImage == "" {
		return nil, errors.Errorf("KUBE_RBAC_PROXY_IMAGE env variable must be set")
	}
	config.kubeRbacProxyImage.repo, config.kubeRbacProxyImage.tag = getImageNameTag(kubeRbacProxyImage)
	err = validateFRRK8SConfig(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}

func validateFRRK8SConfig(c *frrK8SChartConfig) error {
	if c.isOpenShift && !c.enableServiceMonitor {
		return fmt.Errorf("service monitors are required on OpenShift")
	}

	return nil
}

// GetObjects retrieve manifests from chart after patching custom values passed in crdConfig
// and environment variables.
func (h *FRRK8SChart) GetObjects(crdConfig *metallbv1beta1.MetalLB, withPrometheus bool) ([]*unstructured.Unstructured, error) {
	chartValueOpts := &values.Options{}
	chartValues, err := chartValueOpts.MergeValues(getter.All(h.envSettings))
	if err != nil {
		return nil, err
	}

	h.config.patchToChartValues(crdConfig, withPrometheus, chartValues)
	release, err := h.client.Run(h.chart, chartValues)
	if err != nil {
		return nil, err
	}
	objs, err := parseManifest(release.Manifest)
	if err != nil {
		return nil, err
	}
	for _, obj := range objs {
		// Set namespace explicitly into non cluster-scoped resource because helm doesn't
		// patch namespace into manifests at client.Run.
		objKind := obj.GetKind()
		if objKind != "PodSecurityPolicy" {
			obj.SetNamespace(h.namespace)
		}
		// we need to override the security context as helm values are added on top
		// of hardcoded ones in values.yaml, so it's not possible to reset runAsUser
		if isFRRK8SWebhookDeployment(obj) && h.config.isOpenShift {
			securityContext := map[string]interface{}{
				"runAsNonRoot": true,
			}
			err := unstructured.SetNestedMap(obj.Object, securityContext, "spec", "template", "spec", "securityContext")
			if err != nil {
				return nil, err
			}
		}
		if isServiceMonitor(obj) && h.config.isOpenShift {
			err := setOcpMonitorFields(obj)
			if err != nil {
				return nil, err
			}
		}
	}
	return objs, nil
}

func (c *frrK8SChartConfig) patchToChartValues(crdConfig *metallbv1beta1.MetalLB, withPrometheus bool, valueMap map[string]interface{}) {
	c.withFRRK8SValues(crdConfig, valueMap)
	c.withPrometheusValues(valueMap)
}

func (c *frrK8SChartConfig) withFRRK8SValues(crdConfig *metallbv1beta1.MetalLB, valueMap map[string]interface{}) {
	frrk8sValueMap := map[string]interface{}{
		"serviceAccount": map[string]interface{}{
			"create": false,
			"name":   "frr-k8s-daemon",
		},
		"frr": map[string]interface{}{
			"image": map[string]interface{}{
				"repository": c.frrImage.repo,
				"tag":        c.frrImage.tag,
			},
			"metricsPort":       c.frrMetricsPort,
			"secureMetricsPort": c.secureFRRMetricsPort,
		},
	}
	if c.frrk8sImage != nil {
		frrk8sValueMap["image"] = map[string]interface{}{
			"repository": c.frrk8sImage.repo,
			"tag":        c.frrk8sImage.tag,
		}
	}
	withCommonValues(crdConfig, frrk8sValueMap)

	valueMap["frrk8s"] = frrk8sValueMap
}

func (c *frrK8SChartConfig) withPrometheusValues(valueMap map[string]interface{}) {
	tlsConfig := map[string]interface{}{
		"insecureSkipVerify": true,
	}
	annotations := map[string]interface{}{}
	tlsSecret := ""

	if c.isOpenShift {
		tlsConfig, annotations, tlsSecret = ocpPromConfigFor("speaker", c.namespace)
	}

	valueMap["prometheus"] = map[string]interface{}{
		"metricsPort":       c.metricsPort,
		"secureMetricsPort": c.secureMetricsPort,
		"serviceMonitor": map[string]interface{}{
			"enabled":     c.enableServiceMonitor,
			"annotations": annotations,
			"tlsConfig":   tlsConfig,
		},
		"rbacProxy": map[string]interface{}{
			"repository": c.kubeRbacProxyImage.repo,
			"tag":        c.kubeRbacProxyImage.tag,
		},
		"serviceAccount":   "foo", // required by the chart, we won't render roles or rolebindings anyway
		"namespace":        "bar",
		"metricsTLSSecret": tlsSecret,
	}
}

func isFRRK8SWebhookDeployment(obj *unstructured.Unstructured) bool {
	return obj.GetKind() == "Deployment" && obj.GetName() == "webhook-server"
}
