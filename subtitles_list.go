package youtube

type Subtitles map[string][]SubtitlesNode

type SubtitlesNode struct {
	LanguageCode string `json:"languageCode"`
	Ext          string `json:"ext"`
	Url          string `json:"url"`
}

func (s Subtitles) GetSubtitleLink(languageCode string) string {
	var url string
	subtitlesNodes, ok := s[languageCode]
	if !ok {
		return url
	}
	for _, node := range subtitlesNodes {
		url = node.Url
		break
	}
	return url
}
