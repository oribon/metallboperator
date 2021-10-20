package render

import (
	"gopkg.in/yaml.v2"

	metallbv1alpha1 "github.com/metallb/metallb-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

type RenderingFailed struct {
	s string
}

func (e RenderingFailed) Error() string {
	return e.s
}

type OperatorConfig struct {
	NameSpace     string
	ConfigMapName string
	DataField     string
	Pools         []metallbv1alpha1.AddressPool
	Peers         []metallbv1alpha1.BGPPeer
}

// Proto holds the protocol we are speaking.
type Proto string

// MetalLB supported protocols.
const (
	BGP    Proto = "bgp"
	Layer2 Proto = "layer2"
)

// OperatorConfigToMetalLB converts the metallb operator collection of configuration
// in a configmap representing the configuration of metallb.
func OperatorConfigToMetalLB(config *OperatorConfig) (*corev1.ConfigMap, error) {
	res := &corev1.ConfigMap{}
	res.Name = config.ConfigMapName
	res.Namespace = config.NameSpace
	data, err := metalLBConfig(config)
	if err != nil {
		return nil, err
	}
	res.Data = map[string]string{config.DataField: data}
	return res, nil
}

// metalLBConfig converts the given set of CRs to the yaml
// required in metallb configmap
func metalLBConfig(data *OperatorConfig) (string, error) {
	res := configFile{}
	res.Pools = make([]addressPool, len(data.Pools))
	for i, p := range data.Pools {
		res.Pools[i] = poolToMetalLB(p)
	}
	res.Peers = make([]peer, len(data.Peers))
	for i, p := range data.Peers {
		res.Peers[i] = peerToMetalLB(p)
	}
	b, err := yaml.Marshal(&res)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func poolToMetalLB(p metallbv1alpha1.AddressPool) addressPool {
	res := addressPool{}
	res.Protocol = Proto(p.Spec.Protocol)
	res.Name = p.Name
	res.Addresses = make([]string, len(p.Spec.Addresses))
	for i, a := range p.Spec.Addresses {
		res.Addresses[i] = a
	}
	// TODO: avoid buggyip is missing
	if p.Spec.AutoAssign != nil && !*p.Spec.AutoAssign {
		res.AutoAssign = p.Spec.AutoAssign
	}
	res.BGPAdvertisements = make([]bgpAdvertisement, len(p.Spec.BGPAdvertisements))
	for i, b := range p.Spec.BGPAdvertisements {
		if b.AggregationLength > 0 {
			res.BGPAdvertisements[i].AggregationLength = &b.AggregationLength
		}
		if b.LocalPref > 0 {
			res.BGPAdvertisements[i].LocalPref = &b.LocalPref
		}
		res.BGPAdvertisements[i].Communities = make([]string, len(b.Communities))
		for j, c := range b.Communities {
			res.BGPAdvertisements[i].Communities[j] = c
		}
	}

	return res
}

func peerToMetalLB(p metallbv1alpha1.BGPPeer) peer {
	res := peer{}
	res.MyASN = p.Spec.MyASN
	res.ASN = p.Spec.ASN
	res.Addr = p.Spec.Address
	res.SrcAddr = p.Spec.SrcAddress
	res.Port = p.Spec.Port
	if p.Spec.HoldTime > 0 {
		res.HoldTime = p.Spec.HoldTime.String()
	}
	res.RouterID = p.Spec.RouterID
	res.Password = p.Spec.Password
	res.NodeSelectors = make([]nodeSelector, len(p.Spec.NodeSelectors))
	for i, s := range p.Spec.NodeSelectors {
		res.NodeSelectors[i].MatchLabels = make(map[string]string)
		for k, v := range s.MatchLabels {
			res.NodeSelectors[i].MatchLabels[k] = v
		}

		res.NodeSelectors[i].MatchExpressions = make([]selectorRequirements, len(s.MatchExpressions))
		for i, m := range s.MatchExpressions {
			res.NodeSelectors[i].MatchExpressions[i].Key = m.Key
			res.NodeSelectors[i].MatchExpressions[i].Operator = m.Operator
			res.NodeSelectors[i].MatchExpressions[i].Values = make([]string, len(m.Values))
			for j, v := range m.Values {
				res.NodeSelectors[i].MatchExpressions[i].Values[j] = v
			}
		}
	}
	return res
}
