package service

import (
	"fmt"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
	"github.com/imrui/xray-pilot/pkg/types"
)

// Explicit client policies are not silently translated into unverified flow suffixes.
// Legacy subscriptions retain their original native-format behavior.
func explicitUDP443(node, user uint) (bool, error) {
	for _, p := range []struct {
		scope string
		id    uint
	}{{"user", user}, {"node", node}, {"global", 0}} {
		b, err := routingBinding(repository.DB, p.scope, p.id)
		if err != nil {
			return false, err
		}
		if b.UDP443Policy == "block" || b.UDP443Policy == "allow" {
			return true, nil
		}
	}
	return false, nil
}
func incompatibleVision(node, user uint, p *entity.InboundProfile) (bool, error) {
	if p == nil || p.Protocol != types.ProtocolVlessReality {
		return false, nil
	}
	return explicitUDP443(node, user)
}
func (s *SubscribeService) CompatibilityWarnings(token, format string) ([]string, error) {
	warnings := []string{}
	if format != "clash" && format != "singbox" {
		return warnings, nil
	}
	user, err := s.userRepo.FindByToken(token)
	if err != nil {
		return nil, err
	}
	nodes, err := s.nodeRepo.FindHealthyByGroupIDs(userGroupIDs(user.Groups), false)
	if err != nil {
		return nil, err
	}
	for _, n := range nodes {
		keys, err := s.profileRepo.FindActiveKeysForNode(n.ID)
		if err != nil {
			return nil, err
		}
		for _, key := range keys {
			incompatible, err := incompatibleVision(n.ID, user.ID, key.Profile)
			if err != nil {
				return nil, err
			}
			if incompatible {
				warnings = append(warnings, fmt.Sprintf("node:%d/profile:%d: explicit UDP443 policy not verified for %s; entry excluded", n.ID, key.ProfileID, format))
			}
		}
	}
	return warnings, nil
}
