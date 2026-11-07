package domain

import (
	"strings"
)

// AndroidAppURIPrefix помечает в CredentialsSecret.URIs приложение Android по имени
// пакета: «androidapp://com.github.android». Без префикса «github.com» и
// «com.github.android» не различить.
const AndroidAppURIPrefix = "androidapp://"

// MaxCredentialURIs — сколько сайтов и приложений можно привязать к одному логину.
const MaxCredentialURIs = 20

// ParseURIs разбирает список сайтов и приложений, введённый одной строкой: элементы
// разделяются переводами строк, запятыми или пробелами. Пустые элементы и повторы
// отбрасываются, порядок сохраняется. Сами значения не проверяются: это адрес, домен
// или приложение, как их ввёл пользователь; сравнивает их клиент, который заполняет формы.
func ParseURIs(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	var uris []string
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		key := strings.ToLower(f)
		if seen[key] {
			continue
		}
		seen[key] = true
		uris = append(uris, f)
		if len(uris) == MaxCredentialURIs {
			break
		}
	}
	return uris
}

// MatchURI сообщает, относится ли сайт или приложение uri логина к форме, которую
// заполняют: на сайте webDomain (в браузере) или в приложении packageName (webDomain
// пустой). Сравнение строгое, чтобы логин не достался поддельному сайту:
//   - сайт совпадает, если его домен равен домену uri или является его поддоменом:
//     «github.com» подходит для «github.com» и «gist.github.com», но не для
//     «github.com.evil.ru» и не для «notgithub.com»; «www.» не учитывается;
//   - приложение («androidapp://пакет») — только по точному имени пакета и только вне
//     браузера: в браузере packageName — сам браузер.
//
// Домен без точки в uri (кроме localhost и IPv6) не совпадает ни с чем: иначе «com» подошёл бы
// всем сайтам в зоне .com.
func MatchURI(uri, webDomain, packageName string) bool {
	uri = strings.TrimSpace(uri)
	if len(uri) >= len(AndroidAppURIPrefix) && strings.EqualFold(uri[:len(AndroidAppURIPrefix)], AndroidAppURIPrefix) {
		pkg := strings.TrimSpace(uri[len(AndroidAppURIPrefix):])
		return webDomain == "" && pkg != "" && strings.EqualFold(pkg, strings.TrimSpace(packageName))
	}
	site := normalizeHost(webDomain)
	host := normalizeHost(uriHost(uri))
	// Двоеточие — адрес IPv6.
	if site == "" || host == "" || (!strings.ContainsAny(host, ".:") && host != "localhost") {
		return false
	}
	return site == host || strings.HasSuffix(site, "."+host)
}

// uriHost выделяет домен из адреса: без схемы, учётных данных, порта, пути и параметров.
func uriHost(uri string) string {
	s := uri
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if strings.HasPrefix(s, "[") {
		// IPv6: [::1]:8080
		if i := strings.Index(s, "]"); i >= 0 {
			return s[1:i]
		}
		return ""
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}

// normalizeHost приводит домен к виду для сравнения: нижний регистр, без «www.» и
// завершающей точки.
func normalizeHost(host string) string {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return strings.TrimPrefix(host, "www.")
}
