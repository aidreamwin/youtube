package youtube

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Video struct {
	ID              string
	Title           string
	Description     string
	Author          string
	Duration        time.Duration
	PublishDate     time.Time
	Formats         FormatList
	Thumbnails      Thumbnails
	Subtitles       Subtitles
	DASHManifestURL string // URI of the DASH manifest file
	HLSManifestURL  string // URI of the HLS manifest file
}

const dateFormat = "2006-01-02"

func (v *Video) parseVideoInfo(body []byte) error {
	var prData playerResponseData
	if err := json.Unmarshal(body, &prData); err != nil {
		return fmt.Errorf("unable to parse player response JSON: %w", err)
	}

	if err := v.isVideoFromInfoDownloadable(prData); err != nil {
		return err
	}

	return v.extractDataFromPlayerResponse(prData)
}

func (v *Video) isVideoFromInfoDownloadable(prData playerResponseData) error {
	return v.isVideoDownloadable(prData, false)
}

var playerResponsePattern = regexp.MustCompile(`var ytInitialPlayerResponse\s*=\s*(\{.+?\});`)

func (v *Video) parseVideoPage(body []byte) error {
	initialPlayerResponse := playerResponsePattern.FindSubmatch(body)
	if initialPlayerResponse == nil || len(initialPlayerResponse) < 2 {
		return errors.New("no ytInitialPlayerResponse found in the server's answer")
	}

	var prData playerResponseData
	if err := json.Unmarshal(initialPlayerResponse[1], &prData); err != nil {
		return fmt.Errorf("unable to parse player response JSON: %w", err)
	}

	if err := v.isVideoFromPageDownloadable(prData); err != nil {
		return err
	}

	return v.extractDataFromPlayerResponse(prData)
}

func (v *Video) isVideoFromPageDownloadable(prData playerResponseData) error {
	return v.isVideoDownloadable(prData, true)
}

func (v *Video) isVideoDownloadable(prData playerResponseData, isVideoPage bool) error {
	// Check if video is downloadable
	switch prData.PlayabilityStatus.Status {
	case "OK":
		return nil
	case "LOGIN_REQUIRED":
		// for some reason they use same status message for age-restricted and private videos
		if strings.HasPrefix(prData.PlayabilityStatus.Reason, "This video is private") {
			return ErrVideoPrivate
		}
		return ErrLoginRequired
	}

	if !isVideoPage && !prData.PlayabilityStatus.PlayableInEmbed {
		return ErrNotPlayableInEmbed
	}

	return &ErrPlayabiltyStatus{
		Status: prData.PlayabilityStatus.Status,
		Reason: prData.PlayabilityStatus.Reason,
	}
}

func (v *Video) extractDataFromPlayerResponse(prData playerResponseData) error {
	v.Title = prData.VideoDetails.Title
	v.Description = prData.VideoDetails.ShortDescription
	v.Author = prData.VideoDetails.Author
	v.Thumbnails = prData.VideoDetails.Thumbnail.Thumbnails

	if seconds, _ := strconv.Atoi(prData.Microformat.PlayerMicroformatRenderer.LengthSeconds); seconds > 0 {
		v.Duration = time.Duration(seconds) * time.Second
	}

	if str := prData.Microformat.PlayerMicroformatRenderer.PublishDate; str != "" {
		v.PublishDate, _ = time.Parse(dateFormat, str)
	}

	// Assign Streams
	v.Formats = append(prData.StreamingData.Formats, prData.StreamingData.AdaptiveFormats...)
	if len(v.Formats) == 0 {
		return errors.New("no formats found in the server's answer")
	}

	// Sort formats by bitrate
	sort.SliceStable(v.Formats, v.SortBitrateDesc)

	v.HLSManifestURL = prData.StreamingData.HlsManifestURL
	v.DASHManifestURL = prData.StreamingData.DashManifestURL

	// subtitles
	err := v.parseSubtitles(prData)
	if err != nil {
		return err
	}

	return nil
}

func (v *Video) SortBitrateDesc(i int, j int) bool {
	return v.Formats[i].Bitrate > v.Formats[j].Bitrate
}

func (v *Video) SortBitrateAsc(i int, j int) bool {
	return v.Formats[i].Bitrate < v.Formats[j].Bitrate
}

func (v *Video) parseSubtitles(prData playerResponseData) error {
	v.Subtitles = make(Subtitles)
	if len(prData.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks) <= 0 {
		return nil
	}
	var baseUrl string = prData.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks[0].BaseUrl

	for _, captionTrack := range prData.Captions.PlayerCaptionsTracklistRenderer.CaptionTracks {
		for _, ext := range []string{"srv1", "srv2", "srv3", "ttml", "vtt"} {
			v.Subtitles[captionTrack.LanguageCode] = append(v.Subtitles[captionTrack.LanguageCode], SubtitlesNode{
				Ext: ext,
				Url: baseUrl + "&fmt=" + ext,
			})
		}
	}
	if baseUrl == "" {
		return nil
	}
	for _, language := range prData.Captions.PlayerCaptionsTracklistRenderer.TranslationLanguages {
		if len(v.Subtitles[language.LanguageCode]) > 0 {
			continue
		}
		for _, ext := range []string{"srv1", "srv2", "srv3", "ttml", "vtt"} {
			v.Subtitles[language.LanguageCode] = append(v.Subtitles[language.LanguageCode], SubtitlesNode{
				Ext: ext,
				Url: baseUrl + "&fmt=" + ext + "&tlang=" + language.LanguageCode,
			})
		}
	}
	return nil
}
