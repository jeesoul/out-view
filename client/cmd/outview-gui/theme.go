//go:build !headless_test

package main

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
)

// chineseTheme wraps the default theme and injects a Chinese font loaded
// from the Windows system fonts directory at runtime.
type chineseTheme struct {
	fyne.Theme
	font fyne.Resource
}

func newChineseTheme(base fyne.Theme) fyne.Theme {
	t := &chineseTheme{Theme: base}

	// Prefer plain TTF files — Fyne cannot index into TTC collections.
	candidates := []string{
		filepath.Join(os.Getenv("WINDIR"), "Fonts", "simhei.ttf"),      // 黑体 (TTF, works with Fyne)
		filepath.Join(os.Getenv("WINDIR"), "Fonts", "NotoSansSC-VF.ttf"),
		filepath.Join(os.Getenv("WINDIR"), "Fonts", "simsunb.ttf"),     // 宋体Bold (TTF)
	}

	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			t.font = fyne.NewStaticResource(filepath.Base(path), data)
			break
		}
	}

	return t
}

func (t *chineseTheme) Font(style fyne.TextStyle) fyne.Resource {
	if t.font != nil {
		return t.font
	}
	return t.Theme.Font(style)
}
