// Package lobeui is mypack's thin, version-pinned adapter over Lobe MyGo
// (github.com/lobehub/lobe-mygo, MIT) per spec §08A.1/§08A.6. Screens and
// components call these wrappers, never lobe-mygo directly, so a component
// that fails an adoption gate can be swapped for a base-MyGo implementation
// in one place.
//
// Adoption status (PoC):
//   - Used from Lobe MyGo: Input, TextArea, Select, Segmented, SearchBar,
//     DropdownMenu, Tooltip, Modal (without its footer), ToastHost, Skeleton,
//     Hotkey, ActionIcon, CopyButton and the Lucide icon set.
//   - Replaced (fallback on base MyGo): Switch, Checkbox and Progress, because
//     Lobe's primary token is monochrome and cannot be re-themed to
//     mypack's restrained blue accent. Buttons live in components.
package lobeui

import (
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/overlay"
	"github.com/lobehub/lobe-mygo/pkg/theme"

	"mypack.dev/mypack/internal/desktop/design"
)

// Size mirrors theme.Size without leaking the type.
type Size int

const (
	Middle Size = iota
	Small
	Large
)

func (s Size) lobe() theme.Size {
	switch s {
	case Small:
		return theme.Small
	case Large:
		return theme.Large
	}
	return theme.Middle
}

// InputOpts configures Input.
type InputOpts struct {
	Placeholder string
	Prefix      *ui.SVG
	Suffix      *ui.SVG
	Invalid     bool
	Size        Size
}

// Input is a single-line text field. It returns the text input element.
func Input(c *ui.Context, value *string, o InputOpts) ui.Element {
	return field.Input(c, value, field.InputProps{
		Variant: field.InputFilled, Size: o.Size.lobe(), Placeholder: o.Placeholder,
		Prefix: o.Prefix, Suffix: o.Suffix, Invalid: o.Invalid,
	})
}

// TextArea is a multi-line text field.
func TextArea(c *ui.Context, value *string, placeholder string, minRows, maxRows int) ui.Element {
	return field.TextArea(c, value, field.TextAreaProps{
		Variant: field.InputFilled, Placeholder: placeholder, MinRows: minRows, MaxRows: maxRows,
	})
}

// Option is one Select choice.
type Option struct {
	Value, Label string
	Icon         *ui.SVG
	Disabled     bool
}

// Select chooses one option into *selected. It returns the trigger.
func Select(c *ui.Context, selected *string, options []Option, placeholder string, disabled bool) ui.Element {
	opts := make([]field.SelectOption, len(options))
	for i, o := range options {
		opts[i] = field.SelectOption{Value: o.Value, Label: o.Label, Icon: o.Icon, Disabled: o.Disabled}
	}
	return field.Select(c, selected, field.SelectProps{
		Options: opts, Placeholder: placeholder, Variant: field.InputFilled, Disabled: disabled,
	})
}

// segMatch pairs the sliding indicator across frames.
type segMatch struct{ selected *int }

// Segmented is a segmented control over labels; *selected is an index.
// Built on base MyGo with mypack tokens (Lobe's track uses the layout
// color, which reads as a black slab on mypack's charcoal surfaces).
func Segmented(c *ui.Context, selected *int, labels []string, size Size) ui.Element {
	p := design.Of(c)
	h, pad, fs := float32(28), float32(12), design.FontBody
	if size == Small {
		h, pad, fs = 24, 10, design.FontSmall
	}
	parts := ui.SegmentedBase(c, selected, len(labels))
	track := parts.Track.Padding(2).Gap(2).Radius(design.RadiusControl + 1).Background(p.Hover).AlignSelf(ui.Start)
	slide := ui.ElementTransition{Duration: design.Dur(c, design.MotionSlow), Ease: ui.CubicBezier(0.3, 0.7, 0.2, 1), Match: segMatch{selected}}
	track.Children(func() {
		for i, l := range labels {
			seg := parts.Segment(i).Height(h).PaddingX(pad).Radius(design.RadiusSM).FontSize(fs).
				FontWeight(design.WeightMedium).Cursor(ui.CursorPointer)
			fg := p.TextSecondary
			if i == *selected || seg.Hovered() {
				fg = p.Text
			}
			seg.TextColor(fg)
			seg.Children(func() {
				if i == *selected {
					ind := ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(design.RadiusSM).ZIndex(-1).Transition(slide)
					if p.Dark {
						ind.Background(p.SurfaceRaised.Mix(p.Text, 0.06)).Shadow(0, 1, 2, 0, p.Shadow)
					} else {
						ind.Background(p.Surface).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.1)).Border(0.5, p.Border)
					}
				}
				ui.Text(c, l).SingleLine()
			})
		}
	})
	return track
}

// SearchBar is a search field; shortKey ("mod+f") focuses it window-wide.
func SearchBar(c *ui.Context, query *string, placeholder, shortKey string) ui.Element {
	return field.SearchBar(c, query, field.SearchBarProps{
		Placeholder: placeholder, ShortKey: shortKey, Variant: field.InputFilled, Size: theme.Middle,
	})
}

// MenuItem is one row of a dropdown or context menu.
type MenuItem struct {
	Key, Label string
	Icon       *ui.SVG
	Hotkey     string
	Danger     bool
	Disabled   bool
	Divider    bool
}

func menuItems(items []MenuItem) []overlay.MenuItem {
	out := make([]overlay.MenuItem, len(items))
	for i, it := range items {
		out[i] = overlay.MenuItem{Key: it.Key, Label: it.Label, Icon: it.Icon, Hotkey: it.Hotkey,
			Danger: it.Danger, Disabled: it.Disabled, Divider: it.Divider}
	}
	return out
}

// DropdownMenu shows items below a trigger; it returns the chosen key or "".
func DropdownMenu(c *ui.Context, open *bool, items []MenuItem, trigger func(t ui.Element)) string {
	return overlay.DropdownMenu(c, open, overlay.DropdownMenuProps{Items: menuItems(items)}, trigger)
}

// StyleIconTrigger gives a menu trigger the look of an icon button.
func StyleIconTrigger(c *ui.Context, t ui.Element, svg *ui.SVG, title string, active bool) ui.Element {
	return action.StyleActionIcon(c, t, svg, action.ActionIconProps{Size: theme.Small, BlockSize: 28, GlyphSize: 16, Title: title, Active: active})
}

// Tooltip attaches a tooltip (with optional hotkey hint like "mod+n").
func Tooltip(c *ui.Context, anchor ui.Element, text, hotkey string) {
	overlay.Tooltip(c, anchor, text, overlay.TooltipProps{Placement: overlay.Bottom, Hotkey: hotkey})
}

// ActionIcon is a borderless icon-only button with a tooltip title.
func ActionIcon(c *ui.Context, svg *ui.SVG, title string) ui.Element {
	return action.ActionIcon(c, svg, action.ActionIconProps{BlockSize: 28, GlyphSize: 16, Title: title})
}

// CopyButton copies content and confirms.
func CopyButton(c *ui.Context, content string) ui.Element {
	return action.CopyButton(c, content, action.CopyButtonProps{Size: theme.Small})
}

// ToastHost renders toasts; call once per window view.
func ToastHost(c *ui.Context) ui.Element {
	return feedback.ToastHost(c, feedback.ToastHostProps{Placement: feedback.ToastBottomRight})
}

// ToastKind picks a toast icon.
type ToastKind int

const (
	ToastPlain ToastKind = iota
	ToastSuccess
	ToastError
	ToastInfo
)

// Toast shows a transient message.
func Toast(c *ui.Context, title, description string, kind ToastKind) string {
	t := feedback.ToastDefault
	switch kind {
	case ToastSuccess:
		t = feedback.ToastSuccess
	case ToastError:
		t = feedback.ToastError
	case ToastInfo:
		t = feedback.ToastInfo
	}
	return feedback.ShowToast(c, feedback.ToastProps{Title: title, Description: description, Type: t})
}

// ToastAction shows a toast with one action button (e.g. Undo).
func ToastAction(c *ui.Context, title, actionLabel string, fn func()) string {
	return feedback.ShowToast(c, feedback.ToastProps{Title: title, Action: actionLabel, OnAction: fn, Timeout: 8 * time.Second})
}

// Modal shows content in a dialog; the caller draws its own footer so the
// primary button uses mypack's accent.
func Modal(c *ui.Context, open *bool, title string, width float32, content func()) {
	feedback.Modal(c, open, feedback.ModalProps{Title: title, Width: width, NoFooter: true}, content)
}

// Skeleton holds the place of loading content.
func Skeleton(c *ui.Context, w, h float32) ui.Element {
	return display.Skeleton(c, display.SkeletonProps{Width: w, Height: h})
}

// Hotkey shows a key combination ("mod+n") in key caps.
func Hotkey(c *ui.Context, keys string) ui.Element {
	return display.Hotkey(c, keys, display.HotkeyProps{Variant: display.HotkeyBorderless})
}

// Switch is a toggle on base MyGo using mypack's accent (fallback for
// Lobe's monochrome switch).
func Switch(c *ui.Context, on *bool, label string) ui.Element {
	p := design.Of(c)
	track := ui.SwitchBase(c, on).Size(32, 19).Padding(2).Radius(100).Cursor(ui.CursorPointer).
		Transition(design.Transition(c)).Label(label)
	bg := p.BorderStrong
	if *on {
		bg = p.Accent
	}
	if track.IsDisabled() {
		track.Opacity(0.5)
	}
	track.Background(bg)
	x := float32(0)
	if *on {
		x = 13
	}
	track.Children(func() {
		th := ui.Box(c).Size(15, 15).Radius(7.5).Background(ui.RGB(255, 255, 255)).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.22))
		th.Translate(th.AnimateTo("x", x, ui.Animation{Duration: design.Dur(c, design.MotionBase), Ease: ui.CubicBezier(0.3, 0.7, 0.2, 1)}), 0)
	})
	return track
}

// Checkbox is base MyGo's checkbox under mypack's theme.
func Checkbox(c *ui.Context, checked *bool, label string) ui.Element {
	return ui.Checkbox(c, checked, label)
}

// Progress is a thin progress bar; value < 0 is indeterminate.
func Progress(c *ui.Context, value float64) ui.Element {
	p := design.Of(c)
	track := ui.Box(c).Height(4).FillWidth().Radius(2).Background(p.Hover).Clip()
	track.Children(func() {
		if value < 0 {
			bar := ui.Box(c).Height(4).WidthPercent(30).Radius(2).Background(p.Accent)
			t := bar.Loop("indeterminate", 1400*time.Millisecond, ui.CubicBezier(0.45, 0, 0.55, 1))
			bar.TranslatePercent(-100+t*430, 0)
			return
		}
		v := float32(max(0, min(1, value)))
		bar := ui.Box(c).Height(4).Radius(2).Background(p.Accent)
		bar.WidthPercent(bar.AnimateTo("w", v*100, ui.Animation{Duration: design.Dur(c, design.MotionSlow), Ease: ui.CubicBezier(0.25, 0.1, 0.25, 1)}))
	})
	return track.Role(ui.RoleProgress).Range(0, 1, max(0, value))
}
