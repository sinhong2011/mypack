package lobeui

import (
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
)

// Icons used by mypack (Lucide, ISC license via Lobe MyGo). Each returns a
// cached *ui.SVG.
var (
	IconLibrary     = lucide.LayoutGrid
	IconDiscover    = lucide.Compass
	IconBuilds      = lucide.Hammer
	IconRuntimes    = lucide.HardDrive
	IconSettings    = lucide.Settings
	IconPlus        = lucide.Plus
	IconSearch      = lucide.Search
	IconLink        = lucide.Link
	IconGlobe       = lucide.Globe
	IconFolder      = lucide.Folder
	IconFolderOpen  = lucide.FolderOpen
	IconFile        = lucide.FileCode
	IconImage       = lucide.Image
	IconChevronDown = lucide.ChevronDown
	IconChevronRt   = lucide.ChevronRight
	IconCheck       = lucide.Check
	IconClose       = lucide.X
	IconCopy        = lucide.Copy
	IconRefresh     = lucide.RefreshCw
	IconEdit        = lucide.Pencil
	IconTrash       = lucide.Trash2
	IconMore        = lucide.Ellipsis
	IconPlay        = lucide.Play
	IconDownload    = lucide.Download
	IconShield      = lucide.ShieldCheck
	IconWarning     = lucide.TriangleAlert
	IconError       = lucide.CircleAlert
	IconInfo        = lucide.Info
	IconExternal    = lucide.ArrowUpRight
	IconGrid        = lucide.LayoutGrid
	IconList        = lucide.List
	IconClock       = lucide.Clock
	IconTerminal    = lucide.SquareTerminal
	IconLaptop      = lucide.Laptop
	IconMonitor     = lucide.Monitor
	IconPackage     = lucide.Package
	IconLoader      = lucide.LoaderCircle
	IconSuccess     = lucide.CircleCheck
	IconWrench      = lucide.Wrench
	IconRetry       = lucide.RotateCcw
	IconUndo        = lucide.Undo2
	IconLock        = lucide.Lock
	IconKeyboard    = lucide.Keyboard
	IconLanguages   = lucide.Languages
	IconPalette     = lucide.Paintbrush
	IconPlug        = lucide.Plug
	IconLog         = lucide.FileText
	IconArrowRight  = lucide.ArrowRight
	IconStop        = lucide.Square
	IconLayers      = lucide.Layers
	IconWindow      = lucide.AppWindow
	IconRocket      = lucide.Rocket
	IconFailed      = lucide.CircleX
	IconFiles       = lucide.Files
	IconSliders     = lucide.SlidersHorizontal
	IconPaste       = lucide.ClipboardPaste
	IconNetwork     = lucide.Network
	IconSun         = lucide.Sun
	IconMoon        = lucide.Moon
	IconPending     = lucide.CircleDashed
	IconActivity    = lucide.Activity
	IconInbox       = lucide.Inbox
	IconUpdate      = lucide.CircleArrowUp
	IconCode        = lucide.Code
	IconImport      = lucide.Import
	IconStethoscope = lucide.Stethoscope
	IconPower       = lucide.Power
)

// Glyph draws an icon at size in color.
func Glyph(c *ui.Context, svg *ui.SVG, size float32, color ui.Color) ui.Element {
	return ui.Icon(c, svg).FontSize(size).TextColor(color)
}
