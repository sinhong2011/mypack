// Package kit is mypack's own motion-first component library on base MyGo
// ui: buttons, fields, switches, segmented controls, grouped rows, sheets,
// toasts, progress, app icons and the build scene. Every component reads
// design tokens only and has its motion built in (hover, press, focus,
// enter/exit, state changes) through one engine (motion.go), honouring
// Reduce Motion. Components render state handed to them and report user
// intent; they never call the application service.
package kit

import (
	"runtime"
	"strings"

	"github.com/egoist/mygo/ui"

	"mypack.dev/mypack/internal/desktop/design"
)

// LargeTitle is the Display style (28 bold, tight tracking).
func LargeTitle(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontDisplay).FontWeight(design.WeightBold).TextColor(design.Of(c).Text).
		LetterSpacing(design.TrackDisplay).Role(ui.RoleHeading)
}

// Eyebrow is a small uppercase label over a section (10 semibold, open
// tracking), as editorial layouts set them.
func Eyebrow(c *ui.Context, s string) ui.Element {
	return ui.Text(c, strings.ToUpper(s)).FontSize(design.FontMicro + 0.5).FontWeight(design.WeightSemibold).
		TextColor(design.Of(c).TextTertiary).LetterSpacing(design.TrackEyebrow).Role(ui.RoleHeading)
}

// PageTitle is the Title style (20 semibold).
func PageTitle(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontTitle1).FontWeight(design.WeightSemibold).TextColor(design.Of(c).Text).
		LetterSpacing(design.TrackTitle).Role(ui.RoleHeading)
}

// Heading is a section heading (15 semibold).
func Heading(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontTitle3).FontWeight(design.WeightSemibold).TextColor(design.Of(c).Text).Role(ui.RoleHeading)
}

// Headline is the Headline style (13 semibold).
func Headline(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontBody).FontWeight(design.WeightSemibold).TextColor(design.Of(c).Text)
}

// Body is primary body text.
func Body(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontBody).TextColor(design.Of(c).Text)
}

// Secondary is secondary-label body text.
func Secondary(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontBody).TextColor(design.Of(c).TextSecondary)
}

// Caption is small secondary text (11).
func Caption(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontCaption).TextColor(design.Of(c).TextSecondary)
}

// Footnote is 12 pt tertiary text.
func Footnote(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(design.FontSmall).TextColor(design.Of(c).TextTertiary)
}

// Overline is a section header above grouped content.
func Overline(c *ui.Context, s string) ui.Element { return Eyebrow(c, s) }

// SectionHeader is a library section title with an optional count.
func SectionHeader(c *ui.Context, title, count string) ui.Element {
	p := design.Of(c)
	return ui.Row(c).Gap(8).AlignItems(ui.Baseline).Children(func() {
		ui.Text(c, title).FontSize(design.FontTitle2).FontWeight(design.WeightBold).TextColor(p.Text).LetterSpacing(design.TrackTitle).Role(ui.RoleHeading)
		if count != "" {
			ui.Text(c, count).FontSize(design.FontBody).TextColor(p.TextTertiary).FontFeatures("tnum")
		}
	})
}

// Mono is monospaced text (paths, hashes, logs).
func Mono(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).Font(MonoFont).FontSize(design.FontSmall).TextColor(design.Of(c).TextSecondary)
}

// MonoFont is the platform's monospaced UI font.
var MonoFont = func() string {
	switch runtime.GOOS {
	case "darwin":
		return "Menlo"
	case "windows":
		return "Cascadia Mono"
	}
	return "monospace"
}()

// HotkeyText renders "mod+shift+n" the platform way: ⇧⌘N on macOS,
// Ctrl+Shift+N elsewhere.
func HotkeyText(keys string) string {
	if keys == "" {
		return ""
	}
	parts := strings.Split(strings.ToLower(keys), "+")
	mac := runtime.GOOS == "darwin"
	var mods, key string
	for _, k := range parts {
		switch k {
		case "mod", "cmd":
			if mac {
				mods += "⌘"
			} else {
				mods += "Ctrl+"
			}
		case "shift":
			if mac {
				mods = "⇧" + mods
			} else {
				mods += "Shift+"
			}
		case "alt", "opt":
			if mac {
				mods = "⌥" + mods
			} else {
				mods += "Alt+"
			}
		default:
			key = strings.ToUpper(k)
		}
	}
	return mods + key
}
