package main

import (
	"regexp"
	"strings"
)

// SkinConfig holds the dynamic theme/skin settings stored in go_options.
// Shared by both blog_app (main.go) and admin_app (admin.go).
// NOTE: There are no dark-theme-specific fields in SkinConfig.
// Dark theme CSS variables are defined in style.css only.
type SkinConfig struct {
	Theme               string
	ThemeBase           string
	PrimaryColor        string
	PrimaryHover        string
	SuccessColor        string
	TextPrimary         string
	TextSecondary       string
	TextMuted           string
	BgPrimary           string
	BgSecondary         string
	BgAccent            string
	BorderLight         string
	HeaderBg            string
	ThemeBtnHoverBg     string
	ThemeBtnActiveBg    string
	Radius              string
	LayoutContainerMax  string
	LayoutContainerPad  string
	LayoutColumnGap     string
	LayoutPagePadding   string
	LayoutPostPadding   string
	LayoutWidgetPadding string
}

var (
	skinThemeNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	skinColorPattern     = regexp.MustCompile(`(?i)^(#[0-9a-f]{3}|#[0-9a-f]{6}|#[0-9a-f]{8}|rgba?\([0-9.,%\s/+-]+\)|hsla?\([0-9.,%\s/+-]+\)|transparent|inherit|initial|unset|currentColor|[a-z]+)$`)
	skinLengthPattern    = regexp.MustCompile(`(?i)^(0|[0-9]+(?:\.[0-9]+)?)(px|rem|em|vw|vh|%)?$`)
)

func sanitizeThemeName(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || !skinThemeNamePattern.MatchString(value) {
		return fallback
	}
	return value
}

func sanitizeSkinColor(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || !skinColorPattern.MatchString(value) {
		return fallback
	}
	return value
}

func sanitizeSkinLength(value, fallback string) string {
	value = strings.TrimSpace(value)
	match := skinLengthPattern.FindStringSubmatch(value)
	if match == nil {
		return fallback
	}
	if match[1] == "0" || match[2] != "" {
		return strings.ToLower(value)
	}
	return match[1] + "px"
}
