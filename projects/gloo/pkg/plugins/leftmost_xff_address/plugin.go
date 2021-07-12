package leftmost_xff_address

import (
	"github.com/rotisserie/eris"
	v1 "github.com/solo-io/gloo/projects/gloo/pkg/api/v1"
	"github.com/solo-io/gloo/projects/gloo/pkg/plugins"
)

const (
	ErrEnterpriseOnly = "Could not load leftmost_xff_address plugin - this is an Enterprise feature"
	ExtensionName     = "leftmost_xff_address"
)

type plugin struct {
}

var (
	_ plugins.Plugin           = new(plugin)
	_ plugins.HttpFilterPlugin = new(plugin)
	_ plugins.Upgradable       = new(plugin)
)

func NewPlugin() *plugin {
	return &plugin{}
}

func (p *plugin) IsUpgrade() bool {
	return false
}

func (p *plugin) PluginName() string {
	return ExtensionName
}

func (p *plugin) Init(params plugins.InitParams) error {
	return nil
}

func (p *plugin) HttpFilters(params plugins.Params, listener *v1.HttpListener) ([]plugins.StagedHttpFilter, error) {
	if listener.GetOptions().GetLeftmostXffAddress() != nil {
		return nil, eris.New(ErrEnterpriseOnly)
	}
	return nil, nil
}
