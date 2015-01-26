package main

import (
	"net/http"
	"strings"
)

const SEARCH_META_LIMIT = 10 * 1024

func getEncoding(resp *http.Response, body []byte, defaultEnc string) string {
	s := resp.Header.Get("Content-Type")
	if s != "" {
		s = getEncodingContentType(s)
		if s != "" {
			return s
		}
	}

	s = getEncodingBody(body)
	if s != "" {
		return s
	}

	return defaultEnc
}

func getEncodingContentType(s string) string {
	const CHARSET = "charset="
	v := strings.Split(s, ";")
	for i := range v {
		ss := strings.TrimSpace(v[i])
		if strings.HasPrefix(ss, CHARSET) {
			return ss[len(CHARSET):]
		}
	}
	return ""
}

func skipSpaces(s []byte) []byte {
	for i := range s {
		switch s[i] {
		case ' ':
			fallthrough
		case '\n':
			fallthrough
		case '\t':
		default:
			return s[i:]
		}
	}
	return []byte{}
}

func readId(s []byte) (string, []byte) {
	for i := range s {
		switch s[i] {
		case ' ':
			fallthrough
		case '\n':
			fallthrough
		case '\t':
			return string(s[:i]), s[i:]
		}
	}
	return string(s), []byte{}
}

func readString(s []byte, delim byte) (string, []byte) {
	escaped := false
	for i := range s {
		if escaped {
			escaped = false
			continue
		}
		if s[i] == '\\' {
			escaped = true
		} else if s[i] == delim {
			return string(s[:i]), s[i:]
		}
	}
	return string(s), []byte{}
}

func parseTag(s []byte) map[string]string {
	r := map[string]string{}
	for {
		s = skipSpaces(s)
		if len(s) == 0 {
			return r
		}

		if (len(s) >= 2) && (s[0] == '/') && (s[1] == '>') {
			return r
		}

		if s[0] == '>' {
			return r
		}

		id, s := readId(s)
		skipSpaces(s)
		if len(s) == 0 {
			return r
		}
		if s[0] != '=' {
			return r
		}
		s = s[1:]
		s = skipSpaces(s)

		if len(s) == 0 {
			return r
		}

		var k string
		switch s[0] {
		case '\'':
			k, s = readString(s[1:], '\'')
		case '"':
			k, s = readString(s[1:], '"')
		default:
			k, s = readId(s)
		}

		r[strings.ToLower(id)] = strings.ToLower(k)
	}
}

func getEncodingBody(body []byte) string {
	for i := range body {
		if i > SEARCH_META_LIMIT {
			return ""
		}

		if (i + 6) >= len(body) {
			return ""
		}

		if body[i] != '<' || body[i+1] != 'm' || body[i+2] != 'e' || body[i+3] != 't' || body[i+4] != 'a' || body[i+5] != ' ' {
			break
		}

		metaTag := parseTag(body[i+6:])

		if cs, ok := metaTag["charset"]; ok {
			return cs
		}

		if _, ok := metaTag["http-equiv"]; ok {
			if ct, ok := metaTag["content"]; ok {
				return getEncodingContentType(ct)
			}
		}
	}

	return ""
}
