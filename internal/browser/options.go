package browser

import (
	"github.com/chromedp/chromedp"
)

// ChromeOptions returns ChromeDP allocator options optimized for low memory usage.
// Based on research.md findings for Raspberry Pi deployment.
func ChromeOptions(chromePath string, viewportWidth, viewportHeight int, insecureSkipVerify bool, hostResolverRules string) []chromedp.ExecAllocatorOption {
	opts := []chromedp.ExecAllocatorOption{
		// Essential for containers
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),

		// Memory reduction flags
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-background-timer-throttling", true),
		chromedp.Flag("disable-backgrounding-occluded-windows", true),
		chromedp.Flag("disable-renderer-backgrounding", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-translate", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-hang-monitor", true),
		chromedp.Flag("disable-component-extensions-with-background-pages", true),

		// Viewport size
		chromedp.WindowSize(viewportWidth, viewportHeight),

		// Headless mode
		chromedp.Headless,
	}

	// Host resolver rules for mDNS/local hostname resolution (e.g., "MAP grafana.local 192.168.1.100")
	if hostResolverRules != "" {
		opts = append(opts, chromedp.Flag("host-resolver-rules", hostResolverRules))
	}

	// Use custom Chrome path if specified
	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}

	// Ignore certificate errors for self-signed certs
	if insecureSkipVerify {
		opts = append(opts, chromedp.Flag("ignore-certificate-errors", true))
	}

	return opts
}

// DefaultChromeOptions returns options with default viewport.
func DefaultChromeOptions(chromePath string) []chromedp.ExecAllocatorOption {
	return ChromeOptions(chromePath, 1920, 1080, false, "")
}
