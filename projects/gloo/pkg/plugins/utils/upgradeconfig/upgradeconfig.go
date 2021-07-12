package upgradeconfig

import (
	envoy_config_route_v3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyhttp "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/hashicorp/go-multierror"
	errors "github.com/rotisserie/eris"
)

const (
	WebSocketUpgradeType = "websocket"
)

func ValidateHCMUpgradeConfigs(upgradeConfigs []*envoyhttp.HttpConnectionManager_UpgradeConfig) error {
	uniqConfigs := map[string]bool{}
	var multiErr *multierror.Error

	for _, config := range upgradeConfigs {
		if _, ok := uniqConfigs[config.UpgradeType]; ok {
			multiErr = multierror.Append(multiErr, errors.Errorf("upgrade config %s is not unique", config.UpgradeType))
		}
		uniqConfigs[config.UpgradeType] = true
	}
	return multiErr.ErrorOrNil()
}

func ValidateRouteUpgradeConfigs(upgradeConfigs []*envoy_config_route_v3.RouteAction_UpgradeConfig) error {
	uniqConfigs := map[string]bool{}
	var multiErr *multierror.Error

	for _, config := range upgradeConfigs {
		if _, ok := uniqConfigs[config.UpgradeType]; ok {
			multiErr = multierror.Append(multiErr, errors.Errorf("upgrade config %s is not unique", config.UpgradeType))
		}
		uniqConfigs[config.UpgradeType] = true
	}
	return multiErr.ErrorOrNil()
}
