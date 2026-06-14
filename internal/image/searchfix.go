package image

import (
	"context"
)

type ProcessResult struct {
	Localize LocalizeSummary   `json:"localize"`
	Search   []SearchFixResult `json:"search,omitempty"`
}

func ProcessContent(ctx context.Context, content string) (string, ProcessResult) {
	localizer := &ImageLocalizer{}
	localized, summary := localizer.Localize(ctx, content)

	fixed, searchResults := FixArticleImages(localized)
	return fixed, ProcessResult{
		Localize: summary,
		Search:   searchResults,
	}
}
