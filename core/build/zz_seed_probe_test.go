package build

import (
	"os"
	"testing"

	corestate "singbox-launcher/core/state"
	"singbox-launcher/core/template"
)

// Проверка на ЖИВОМ шаблоне из бандла: засеются ли каналы при старте.
func TestZZSeedFromBundle(t *testing.T) {
	raw, err := os.ReadFile("/Applications/singbox-launcher.app/Contents/MacOS/bin/wizard_template.json")
	if err != nil {
		t.Skip(err)
	}
	td := &template.TemplateData{RawTemplate: raw}
	defs := td.DefaultChannels()
	t.Logf("default_channels в бандле: %d", len(defs))

	st := &corestate.State{} // как состояние без секции channels
	seeds := make([]corestate.ChannelSeed, 0, len(defs))
	for _, d := range defs {
		seeds = append(seeds, corestate.ChannelSeed{Tag: d.Tag, Label: d.Label, Enabled: d.IsEnabled()})
	}
	if !st.SeedChannels(seeds) {
		t.Fatal("сидирование не сработало")
	}
	for _, c := range st.Channels {
		t.Logf("  засеян %s %q enabled=%v", c.Tag, c.Label, c.Enabled)
	}
}
