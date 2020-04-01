package translator

import (
	errors "github.com/rotisserie/eris"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

func (t *translatorInstance) verifyAuthConfigs(params plugins.Params, reports reporter.ResourceReports) {
	authConfigs := params.Snapshot.AuthConfigs
	for _, ac := range authConfigs {
		configs := ac.GetConfigs()
		if len(configs) == 0 {
			reports.AddError(ac, errors.Errorf("No configurations for auth config %v", ac.Metadata.String()))
		}
	}
}
