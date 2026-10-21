package desktop

import (
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/yazeed1s/quarro/internal/windowprefs"
)

// NewMainWindow creates the main window from the saved window preferences,
// hidden until the frontend has painted.
func NewMainWindow(app *application.App, wp windowprefs.Prefs) *application.WebviewWindow {
	// Startup background colour (last theme's --bg).
	r, g, b, _ := windowprefs.ParseHex(wp.Background)
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   mainWindow,
		Title:  "Quarro",
		Width:  1440,
		Height: 900,
		URL:    "/",
		// Frameless leaves title-bar and window chrome rendering to the
		// frontend. Linux users choose (Settings → Appearance → Title bar);
		// Windows is always frameless; macOS uses the native window below.
		Frameless: windowprefs.Frameless(runtime.GOOS, wp.TitleBar),
		// AllowSimpleEventEmit stays off because the app loads from the asset server and does not need bare unrouted events.
		AllowSimpleEventEmit: false,
		InitialPosition:      application.WindowCentered,
		// Keep native webview pinch and scroll zoom disabled so it does not fight the app-level appearance zoom preference.
		ZoomControlEnabled: false,
		// Stay hidden until the page has loaded (see the show handlers
		// below), so the window never shows the webview's blank white canvas
		// before the app has painted anything. BackgroundColour
		// is the last theme's --bg (the frontend reports it on every theme
		// change) so any residual gap before that first paint matches.
		Hidden:           true,
		BackgroundColour: application.NewRGB(r, g, b),
		Mac: application.MacWindow{
			// No title bar of our own on macOS: the native one is
			// transparent and the content runs under it, so the traffic
			// lights sit inside the session bar. The unified toolbar style
			// makes that title area ~52px tall, centring the lights on the
			// 54px session bar.
			TitleBar: application.MacTitleBarHiddenInsetUnified,
		},
		Windows: application.WindowsWindow{
			// Lets WebView2 treat CSS app-region: drag (the session bar) as a
			// native caption: native drag/snap, double-click maximise, and the
			// right-click window menu. WebView2CompositionHosting would also
			// give the maximise button Windows 11 snap layouts, but in beta.22
			// it stops forwarding OLE drag-and-drop, which the object browser
			// relies on, so it stays off.
			NonClientRegionSupport: true,
		},
		Linux: application.LinuxWindow{
			// A non-nil Linux struct otherwise silently falls back to
			// WebviewGpuPolicyAlways, which keeps WebKit's accelerated-
			// compositing buffers and texture caches resident at all times.
			// OnDemand only pays that cost while content (e.g. echarts
			// canvases) actually needs the GPU path.
			WebviewGpuPolicy: application.WebviewGpuPolicyOnDemand,
		},
	})

	// WindowRuntimeReady fires as soon as the JS<->Go bridge is up, well
	// before the webview has actually painted a frame, so showing on it still
	// exposes a blank canvas. Each platform's load-finished event is much
	// closer to a real painted frame. once keeps the platform event and the
	// fallback below from showing the window twice.
	var once sync.Once
	show := func() {
		once.Do(func() {
			// Windows creates the window with the .exe's icon, which is
			// always the shipped (dark) one. On Linux this repairs a launcher
			// entry a reinstall reset, and is a no-op otherwise.
			if err := setAppIcon(wp.AppIcon); err != nil {
				log.Printf("app icon not applied: %v", err)
			}
			win.Show()
		})
	}
	// Linux's WindowLoadFinished (WEBKIT_LOAD_FINISHED) fires once the
	// document and its resources have loaded; the short extra delay covers
	// WebKitGTK's remaining gap until the compositor presents that frame.
	win.OnWindowEvent(events.Linux.WindowLoadFinished, func(_ *application.WindowEvent) {
		time.AfterFunc(60*time.Millisecond, show)
	})
	win.OnWindowEvent(events.Mac.WebViewDidFinishNavigation, func(_ *application.WindowEvent) {
		show()
	})
	win.OnWindowEvent(events.Windows.WebViewNavigationCompleted, func(_ *application.WindowEvent) {
		show()
	})
	// If no load event ever arrives (a platform event renamed in a Wails
	// upgrade, a navigation error), show the window late rather than leave
	// the app running invisibly.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(_ *application.ApplicationEvent) {
		time.AfterFunc(1500*time.Millisecond, show)
	})
	return win
}
