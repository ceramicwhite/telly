package guideproviders

import (
	"fmt"

	"github.com/tellytv/telly/internal/xmltv"
	"github.com/tellytv/telly/utils"
)

// XMLTV is a GuideProvider supporting XMLTV files.
type XMLTV struct {
	BaseConfig Configuration

	channels []Channel
	file     *xmltv.TV
}

func newXMLTV(config *Configuration) (GuideProvider, error) {
	provider := &XMLTV{BaseConfig: *config}

	if _, loadErr := provider.Refresh(nil); loadErr != nil {
		return nil, loadErr
	}

	return provider, nil
}

// Name returns the name of the GuideProvider.
func (x *XMLTV) Name() string {
	return "XMLTV"
}

// Channels returns a slice of Channel that the provider has available.
func (x *XMLTV) Channels() ([]Channel, error) {
	return x.channels, nil
}

// Schedule returns a slice of xmltv.Programme for the given channelIDs.
func (x *XMLTV) Schedule(inputChannels []Channel, inputProgrammes []ProgrammeContainer) (map[string]interface{}, []ProgrammeContainer, error) {
	channelIDMap := make(map[string]struct{})
	for _, chanID := range inputChannels {
		channelIDMap[chanID.ID] = struct{}{}
	}

	filteredProgrammes := make([]ProgrammeContainer, 0)

	for _, programme := range x.file.Programmes {
		if _, ok := channelIDMap[programme.Channel]; ok {
			filteredProgrammes = append(filteredProgrammes, ProgrammeContainer{programme, nil})
		}
	}

	return nil, filteredProgrammes, nil
}

// Refresh causes the provider to request the latest information.
func (x *XMLTV) Refresh(lineupStateJSON []byte) ([]byte, error) {
	xTV, xTVErr := utils.GetXMLTV(x.BaseConfig.XMLTVURL, false)
	if xTVErr != nil {
		return nil, fmt.Errorf("error when getting XMLTV file: %s", xTVErr)
	}

	x.file = xTV

	for _, channel := range xTV.Channels {
		logos := make([]Logo, 0)

		for _, icon := range channel.Icons {
			logos = append(logos, Logo{
				URL:    icon.Source,
				Height: icon.Height,
				Width:  icon.Width,
			})
		}

		x.channels = append(x.channels, Channel{
			ID:       channel.ID,
			Name:     channel.DisplayNames[0].Value,
			Logos:    logos,
			Number:   channel.LCN,
			CallSign: "UNK",
		})
	}

	return nil, nil
}

// Configuration returns the base configuration backing the provider
func (x *XMLTV) Configuration() Configuration {
	return x.BaseConfig
}