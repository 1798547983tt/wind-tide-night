package service

import (
	"maps"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// wind-tide: the site owns the relay side of the AIS playground pool
// (wind-tide/API.md §15): the "AIS PG" group, the ratios of the AIS models
// and the channel tagged wt-ais that forwards them to aistudio-api. Every
// start puts them back; the owner may change anything else in the admin UI.

// WtEnsureAisChannel creates or repairs the group, the ratios and the
// channel of the AIS playground pool. It does nothing while the pool is not
// configured and is idempotent; one log line says what it did.
func WtEnsureAisChannel() error {
	if !WtAisEnabled() {
		return nil
	}
	var changes []string
	groupRatios := ratio_setting.GetGroupRatioCopy()
	if _, ok := groupRatios[WtAisGroup]; !ok {
		groupRatios[WtAisGroup] = 1
		if err := wtUpdateOptionJSON("GroupRatio", groupRatios); err != nil {
			return err
		}
		changes = append(changes, "group ratio")
	}
	usableGroups := setting.GetUserUsableGroupsCopy()
	if _, ok := usableGroups[WtAisGroup]; !ok {
		usableGroups[WtAisGroup] = WtAisGroupLabel
		if err := wtUpdateOptionJSON("UserUsableGroups", usableGroups); err != nil {
			return err
		}
		changes = append(changes, "usable groups")
	}
	// Each relay model bills like its upstream model; a model without a ratio
	// of its own bills at 1.
	for _, ratios := range []struct {
		option string
		values map[string]float64
	}{
		{"ModelRatio", ratio_setting.GetModelRatioCopy()},
		{"CompletionRatio", ratio_setting.GetCompletionRatioCopy()},
	} {
		changed := false
		for _, spec := range model.WtAisModels {
			want := 1.0
			if upstream, ok := ratios.values[spec.Upstream]; ok {
				want = upstream
			}
			if current, ok := ratios.values[spec.Name]; ok && current == want {
				continue
			}
			ratios.values[spec.Name] = want
			changed = true
		}
		if !changed {
			continue
		}
		if err := wtUpdateOptionJSON(ratios.option, ratios.values); err != nil {
			return err
		}
		changes = append(changes, strings.ToLower(ratios.option))
	}

	mapping, err := common.Marshal(wtAisModelMapping())
	if err != nil {
		return err
	}
	channels, err := model.GetChannelsByTag(WtAisChannelTag, true, true)
	if err != nil {
		return err
	}
	var channel *model.Channel
	for _, candidate := range channels {
		if channel == nil || candidate.Id < channel.Id {
			channel = candidate
		}
	}
	switch {
	case channel == nil:
		channel = &model.Channel{CreatedTime: common.GetTimestamp()}
		channel.SetTag(WtAisChannelTag)
		wtApplyAisChannel(channel, string(mapping))
		if err := channel.Insert(); err != nil {
			return err
		}
		changes = append(changes, "channel created")
	case !wtAisChannelMatches(channel):
		wtApplyAisChannel(channel, string(mapping))
		if err := channel.Update(); err != nil {
			return err
		}
		changes = append(changes, "channel updated")
	}
	if len(changes) > 0 {
		model.InitChannelCache()
	}
	summary := "unchanged"
	if len(changes) > 0 {
		summary = strings.Join(changes, ", ")
	}
	common.SysLog(common.LogText("wt ais: channel %s: %s", WtAisChannelName, summary))
	return nil
}

// wtUpdateOptionJSON stores a map option the way the admin UI saves it.
func wtUpdateOptionJSON(key string, value any) error {
	encoded, err := common.Marshal(value)
	if err != nil {
		return err
	}
	return model.UpdateOption(key, string(encoded))
}

// wtAisModelNames is the channel's model list, relay names in table order.
func wtAisModelNames() string {
	names := make([]string, 0, len(model.WtAisModels))
	for _, spec := range model.WtAisModels {
		names = append(names, spec.Name)
	}
	return strings.Join(names, ",")
}

// wtAisModelMapping maps each relay model to the model aistudio-api serves.
func wtAisModelMapping() map[string]string {
	mapping := make(map[string]string, len(model.WtAisModels))
	for _, spec := range model.WtAisModels {
		mapping[spec.Name] = spec.Upstream
	}
	return mapping
}

// wtApplyAisChannel writes the fields the site owns on the AIS channel.
func wtApplyAisChannel(channel *model.Channel, mapping string) {
	baseURL, priority, weight := wtAisURL(), int64(0), uint(0)
	channel.Type = constant.ChannelTypeGemini
	channel.Key = WtAisAPIKey()
	channel.BaseURL = &baseURL
	channel.Models = wtAisModelNames()
	channel.Group = "default,vip,svip," + WtAisGroup
	channel.ModelMapping = &mapping
	channel.Name = WtAisChannelName
	channel.Status = common.ChannelStatusEnabled
	channel.Priority = &priority
	channel.Weight = &weight
}

// wtAisChannelMatches reports whether the fields the site owns are as the
// site wants them. The mapping is compared as a map, so the order its JSON
// was written in does not matter.
func wtAisChannelMatches(channel *model.Channel) bool {
	var mapping map[string]string
	if common.UnmarshalJsonStr(channel.GetModelMapping(), &mapping) != nil {
		return false
	}
	return channel.Type == constant.ChannelTypeGemini &&
		channel.Key == WtAisAPIKey() &&
		channel.BaseURL != nil && *channel.BaseURL == wtAisURL() &&
		channel.Models == wtAisModelNames() &&
		channel.Group == "default,vip,svip,"+WtAisGroup &&
		maps.Equal(mapping, wtAisModelMapping()) &&
		channel.Name == WtAisChannelName &&
		channel.Status == common.ChannelStatusEnabled &&
		channel.GetPriority() == 0 &&
		channel.GetWeight() == 0
}
