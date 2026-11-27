package botclient

import (
	"slices"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/commons/wire"
)

// Dialog is the last dialog page the server sent the bot: the object it
// belongs to and the bypass commands of its links. The server accepts only
// a bypass that is a link of the last page it sent (network/bypass.go
// bypassWhitelist), so a new page replaces the old one whole.
type Dialog struct {
	ObjectID int32
	Links    []string
}

// Has reports whether cmd is a link of the page.
func (d Dialog) Has(cmd string) bool { return slices.Contains(d.Links, cmd) }

// dialogLinks returns the bypass commands of the links of page html, read
// the way the server's whitelist reads them (bypassWhitelist.record): a
// link is "bypass [-h ]<command>" inside double quotes, trimmed of spaces
// and control characters. A link taking an edit-box value ('$') is left
// out: the bot fills in no edit box.
func dialogLinks(html string) []string {
	const marker = `"bypass `
	var links []string
	for i := 0; i < len(html); {
		start := strings.Index(html[i:], marker)
		if start < 0 {
			break
		}
		start += i
		end := strings.IndexByte(html[start+1:], '"')
		if end < 0 {
			break
		}
		end += start + 1
		start += len(marker)
		if strings.HasPrefix(html[start:], "-h") {
			start += 3
		}
		i = end + 1
		if start > end {
			continue
		}
		cmd := strings.TrimFunc(html[start:end], isCtrlOrSpace)
		if cmd != "" && !strings.Contains(cmd, "$") {
			links = append(links, cmd)
		}
	}
	return links
}

// isCtrlOrSpace matches the characters the server trims off a link: every
// rune up to and including the space.
func isCtrlOrSpace(r rune) bool { return r <= ' ' }

// bypassAction sends bypass command cmd, a link of the open dialog. The
// bypass gate (gateBypass) spaces it.
func bypassAction(cmd string) Action {
	return Action{kind: actBypass, payload: encodeRequestBypassToServer(cmd)}
}

// encodeRequestBypassToServer sends bypass command cmd
// (clientpackets/html.go).
func encodeRequestBypassToServer(cmd string) []byte {
	w := wire.NewPacketWriter(opGameRequestBypassToServer)
	w.WriteString(cmd)
	return w.Bytes()
}
