package openshift

import (
	"context"
	"crypto/tls"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	crctls "github.com/openshift/controller-runtime-common/pkg/tls"
	libgocrypto "github.com/openshift/library-go/pkg/crypto"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TODO(curves): check if controller-runtime-common adds a TLSGroup-to-CurveID
// translation helper so we can reuse it instead of maintaining this map.
// https://github.com/openshift/controller-runtime-common/blob/64ee174f5e2ebc630fbb554dd114d7a7a878693f/pkg/tls/tls.go#L123
var tlsGroupToCurveID = map[configv1.TLSGroup]uint16{
	configv1.TLSGroupX25519:             uint16(tls.X25519),
	configv1.TLSGroupSecP256r1:          uint16(tls.CurveP256),
	configv1.TLSGroupSecP384r1:          uint16(tls.CurveP384),
	configv1.TLSGroupSecP521r1:          uint16(tls.CurveP521),
	configv1.TLSGroupX25519MLKEM768:     uint16(tls.X25519MLKEM768),
	configv1.TLSGroupSecP256r1MLKEM768:  uint16(tls.SecP256r1MLKEM768),
	configv1.TLSGroupSecP384r1MLKEM1024: uint16(tls.SecP384r1MLKEM1024),
}

// TLSConfig holds the resolved OpenShift TLS profile and adherence policy.
type TLSConfig struct {
	CipherSuites     string // IANA names, comma-separated
	CurvePreferences string // numeric CurveIDs, comma-separated
	MinVersion       string // e.g. "VersionTLS12"

	profileSpec configv1.TLSProfileSpec
	adherence   configv1.TLSAdherencePolicy
}

// FetchTLSConfig fetches the cluster-wide TLS security profile and adherence
// policy from apiservers.config.openshift.io/cluster and returns the resolved
// TLS parameters as plain strings.
func FetchTLSConfig(ctx context.Context, scheme *runtime.Scheme, logger logr.Logger) (*TLSConfig, error) {
	cl, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return nil, err
	}

	profileSpec, err := crctls.FetchAPIServerTLSProfile(ctx, cl)
	if err != nil {
		return nil, err
	}

	adherence, err := crctls.FetchAPIServerTLSAdherencePolicy(ctx, cl)
	if err != nil {
		return nil, err
	}

	cipherSuites, curvePreferences, minVersion := tlsStringsFromProfile(profileSpec)
	logger.Info("resolved OpenShift TLS profile",
		"ciphers", cipherSuites, "minVersion", minVersion)

	return &TLSConfig{
		CipherSuites:     cipherSuites,
		CurvePreferences: curvePreferences,
		MinVersion:       minVersion,
		profileSpec:      profileSpec,
		adherence:        adherence,
	}, nil
}

// SetupProfileWatcher registers a controller that watches the APIServer CR for
// TLS profile or adherence policy changes and calls cancel() to trigger a
// graceful operator restart.
func (tc *TLSConfig) SetupProfileWatcher(mgr ctrl.Manager, cancel context.CancelFunc, logger logr.Logger) error {
	watcher := &crctls.SecurityProfileWatcher{
		Client:                    mgr.GetClient(),
		InitialTLSProfileSpec:     tc.profileSpec,
		InitialTLSAdherencePolicy: tc.adherence,
		OnProfileChange: func(_ context.Context, _, _ configv1.TLSProfileSpec) {
			logger.Info("TLS profile changed, restarting operator")
			cancel()
		},
		OnAdherencePolicyChange: func(_ context.Context, _, _ configv1.TLSAdherencePolicy) {
			logger.Info("TLS adherence policy changed, restarting operator")
			cancel()
		},
	}
	return watcher.SetupWithManager(mgr)
}

func tlsStringsFromProfile(spec configv1.TLSProfileSpec) (string, string, string) {
	var cipherSuites string
	if len(spec.Ciphers) > 0 {
		cipherSuites = strings.Join(libgocrypto.OpenSSLToIANACipherSuites(spec.Ciphers), ",")
	}
	var curveIDs []string
	for _, g := range spec.Groups {
		if id, ok := tlsGroupToCurveID[g]; ok {
			curveIDs = append(curveIDs, strconv.FormatUint(uint64(id), 10))
		}
	}
	curvePreferences := strings.Join(curveIDs, ",")
	minVersion := string(spec.MinTLSVersion)
	return cipherSuites, curvePreferences, minVersion
}
