// SPDX-License-Identifier: GPL-3.0-or-later

// Source parsing reads a source's content into named nodes without network
// I/O. Each content format is one registered parser.
package subscription

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"net"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SourceFormat is a content format, or auto to recognize it.
type SourceFormat string

const (
	SourceFormatAuto       SourceFormat = "auto"
	SourceFormatSingBox    SourceFormat = "sing-box"
	SourceFormatMihomo     SourceFormat = "mihomo"
	SourceFormatShareLinks SourceFormat = "share-links"
)

// SourceItem is one node a source's content holds.
type SourceItem struct {
	// Location is where the node sits in the content, such as outbounds[3],
	// proxies[7] or line 12.
	Location string
	Name     string
	// Protocol is the sing-box outbound type the node converts to, or the
	// content's own type name when it has none.
	Protocol string
	convert  func() (map[string]any, error)
}

// Convert returns the item's sing-box outbound.
func (item SourceItem) Convert() (map[string]any, error) { return item.convert() }

// SourceParser reads one content format.
type SourceParser interface {
	Format() SourceFormat
	// Sniff scores how likely body is in the parser's format, cheaply: 0 when
	// it cannot be, higher for more certainty.
	Sniff(body []byte) int
	// Items splits body into its nodes in content order. Entries that are not
	// nodes are left out; anything that does not parse fails the whole body.
	Items(body []byte) ([]SourceItem, error)
}

// sourceParsers holds every registered content format, in the order auto
// recognition prefers on equal scores.
var sourceParsers []SourceParser

func registerSourceParser(parser SourceParser) {
	if lookupSourceParser(parser.Format()) != nil || parser.Format() == SourceFormatAuto {
		panic(fmt.Sprintf("subscription source parser %q registered twice", parser.Format()))
	}
	sourceParsers = append(sourceParsers, parser)
}

func lookupSourceParser(format SourceFormat) SourceParser {
	for _, parser := range sourceParsers {
		if parser.Format() == format {
			return parser
		}
	}
	return nil
}

func init() {
	registerSourceParser(singBoxSourceParser{})
	registerSourceParser(mihomoSourceParser{})
	registerSourceParser(shareLinkSourceParser{})
}

// ValidSourceFormat reports whether format names a registered format, or
// auto when allowAuto.
func ValidSourceFormat(format SourceFormat, allowAuto bool) bool {
	return lookupSourceParser(format) != nil || allowAuto && format == SourceFormatAuto
}

// Source parse error codes.
const (
	SourceErrorEmptyContent       = "empty_content"
	SourceErrorContentTooLarge    = "content_too_large"
	SourceErrorUnrecognizedFormat = "unrecognized_format"
	SourceErrorInvalidContent     = "invalid_content"
	SourceErrorTooManyNodes       = "too_many_nodes"
	SourceErrorInvalidNodeName    = "invalid_node_name"
	SourceErrorNodeNotConvertible = "node_not_convertible"
)

// SourceParseError is why content cannot be saved. A node that does not
// convert names its location and name; Reason is a fixed description that
// never repeats values from the content.
type SourceParseError struct {
	Code     string `json:"code"`
	Location string `json:"location,omitempty"`
	Name     string `json:"name,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

func (err *SourceParseError) Error() string {
	message := "subscription source content: " + err.Code
	if err.Location != "" {
		message += " at " + err.Location
	}
	if err.Reason != "" {
		message += ": " + err.Reason
	}
	return message
}

func contentError(location, reason string) error {
	return &SourceParseError{Code: SourceErrorInvalidContent, Location: location, Reason: reason}
}

// SourceReading is a content read into its nodes.
type SourceReading struct {
	Format SourceFormat
	Items  []SourceReadingItem
}

// SourceReadingItem is a node of the content, whether the filter keeps it,
// its key within the source and, when kept, its outbound without a tag.
type SourceReadingItem struct {
	SourceItem
	Kept      bool
	OriginKey string
	Outbound  map[string]any
}

// Kept returns the items the filter keeps.
func (reading SourceReading) Kept() []SourceReadingItem {
	kept := make([]SourceReadingItem, 0, len(reading.Items))
	for _, item := range reading.Items {
		if item.Kept {
			kept = append(kept, item)
		}
	}
	return kept
}

// ReadSource reads content strictly: the content must parse as format, or as
// the format that recognizes it best for auto, after Base64 wrapping is
// removed; the filter then selects nodes by name, and every kept node must
// convert. A failure is a SourceParseError. When the content parses but a
// kept node does not convert, the reading still lists every item.
func ReadSource(content []byte, format SourceFormat, filter SourceFilter) (SourceReading, error) {
	compiled, err := filter.compile()
	if err != nil {
		return SourceReading{}, err
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return SourceReading{}, &SourceParseError{Code: SourceErrorEmptyContent}
	}
	if len(content) > MaximumSourceBodyBytes {
		return SourceReading{}, &SourceParseError{Code: SourceErrorContentTooLarge}
	}
	body := unwrapBase64(content)
	if !utf8.Valid(body) {
		return SourceReading{}, contentError("", "the content is not UTF-8 text")
	}
	parser, err := sourceParser(format, body)
	if err != nil {
		return SourceReading{}, err
	}
	items, err := parser.Items(body)
	if err != nil {
		var parseErr *SourceParseError
		if errors.As(err, &parseErr) {
			return SourceReading{}, err
		}
		return SourceReading{}, contentError("", err.Error())
	}
	if len(items) > MaximumNodes {
		return SourceReading{}, &SourceParseError{Code: SourceErrorTooManyNodes}
	}
	reading := SourceReading{Format: parser.Format(), Items: make([]SourceReadingItem, len(items))}
	keys := originKeys(items)
	for index, item := range items {
		reading.Items[index] = SourceReadingItem{SourceItem: item, Kept: compiled.keep(item.Name), OriginKey: keys[index]}
	}
	for index := range reading.Items {
		item := &reading.Items[index]
		if !item.Kept {
			continue
		}
		if !validSourceNodeName(item.Name) {
			return reading, &SourceParseError{Code: SourceErrorInvalidNodeName, Location: item.Location, Reason: "a name must be 1 to 512 bytes without control characters"}
		}
		outbound, err := convertSourceItem(item.SourceItem)
		if err != nil {
			return reading, &SourceParseError{Code: SourceErrorNodeNotConvertible, Location: item.Location, Name: item.Name, Reason: err.Error()}
		}
		item.Outbound = outbound
	}
	return reading, nil
}

// convertSourceItem converts item and checks the outbound it yields.
func convertSourceItem(item SourceItem) (map[string]any, error) {
	converted, err := item.Convert()
	if err != nil {
		return nil, err
	}
	outbound := maps.Clone(converted)
	outbound["tag"] = item.Name
	if outbound["type"] == "wireguard" {
		if !hasRemoteCoordinate(outbound) {
			return nil, &outboundFieldError{Path: "peers", Code: "required"}
		}
	} else if err := validateSourceOutbound(outbound); err != nil {
		return nil, err
	}
	delete(outbound, "tag")
	return outbound, nil
}

func sourceParser(format SourceFormat, body []byte) (SourceParser, error) {
	if format != SourceFormatAuto {
		parser := lookupSourceParser(format)
		if parser == nil {
			return nil, fmt.Errorf("subscription source format %q is invalid", format)
		}
		return parser, nil
	}
	var best SourceParser
	bestScore := 0
	for _, parser := range sourceParsers {
		if score := parser.Sniff(body); score > bestScore {
			best, bestScore = parser, score
		}
	}
	if best == nil {
		return nil, &SourceParseError{Code: SourceErrorUnrecognizedFormat}
	}
	return best, nil
}

// unwrapBase64 decodes content that is one Base64 text, as many providers
// serve share links, and returns other content unchanged.
func unwrapBase64(content []byte) []byte {
	compact := make([]byte, 0, len(content))
	for _, character := range content {
		switch {
		case character == ' ' || character == '\t' || character == '\r' || character == '\n':
			continue
		case character >= 'A' && character <= 'Z', character >= 'a' && character <= 'z', character >= '0' && character <= '9',
			character == '+', character == '/', character == '-', character == '_', character == '=':
			compact = append(compact, character)
		default:
			return content
		}
	}
	decoded, err := decodeAnyBase64(string(compact))
	if err != nil || len(bytes.TrimSpace(decoded)) == 0 || !utf8.Valid(decoded) {
		return content
	}
	for _, character := range string(decoded) {
		if unicode.IsControl(character) && character != '\n' && character != '\r' && character != '\t' {
			return content
		}
	}
	return decoded
}

// originKeys keys each item by its name, adding #2, #3 and so on to later
// items of the same name.
func originKeys(items []SourceItem) []string {
	keys := make([]string, len(items))
	used := make(map[string]bool, len(items))
	counts := make(map[string]int, len(items))
	for index, item := range items {
		number := counts[item.Name] + 1
		key := item.Name
		if number > 1 {
			key = item.Name + "#" + strconv.Itoa(number)
		}
		for used[key] {
			number++
			key = item.Name + "#" + strconv.Itoa(number)
		}
		counts[item.Name], used[key], keys[index] = number, true, key
	}
	return keys
}

func validSourceNodeName(name string) bool {
	return name != "" && len(name) <= 512 && utf8.ValidString(name) && strings.IndexFunc(name, unicode.IsControl) < 0
}

// unnamedNode names a node without a name by its protocol, server and port.
func unnamedNode(protocol, server string, port int64) string {
	if server == "" {
		return protocol
	}
	return protocol + " " + net.JoinHostPort(server, strconv.FormatInt(port, 10))
}
