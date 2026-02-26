package extractors

import (
	"strings"
)

// GetExtractor returns an appropriate extractor based on the server name or URL
func GetExtractor(serverName string) Extractor {
	serverLower := strings.ToLower(serverName)

	// HD-1, HD-2, HD-3 servers from hianime use megacloud.blog
	// These need the MegaCloud extractor (not decrypt.broggl.farm)
	if strings.Contains(serverLower, "hd-") {
		return NewMegaCloudExtractor()
	}

	// All other servers use videostr.net/streameeeeee.site embeds
	// and work best with decrypt.broggl.farm (VidCloud extractor)
	if strings.Contains(serverLower, "vidcloud") ||
		strings.Contains(serverLower, "upcloud") ||
		strings.Contains(serverLower, "akcloud") ||
		strings.Contains(serverLower, "megacloud") {
		return NewVidCloudExtractor()
	}

	// Default to VidCloud extractor
	return NewVidCloudExtractor()
}
