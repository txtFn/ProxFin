package src

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var directHLSURIAttribute = regexp.MustCompile(`URI="([^"]*)"`)

func rewriteDirectHLSPlaylist(
	channelID string,
	playlistURL string,
	content string,
) (string, error) {
	base, err := url.Parse(playlistURL)
	if err != nil {
		return "", err
	}

	lines := strings.Split(content, "\n")

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			if !strings.Contains(trimmed, `URI="`) {
				continue
			}

			var rewriteErr error

			lines[i] = directHLSURIAttribute.ReplaceAllStringFunc(
				line,
				func(attribute string) string {
					if rewriteErr != nil {
						return attribute
					}

					matches := directHLSURIAttribute.FindStringSubmatch(attribute)
					if len(matches) != 2 {
						return attribute
					}

					proxyURL, err := registerDirectHLSURL(
						channelID,
						base,
						matches[1],
					)
					if err != nil {
						rewriteErr = err
						return attribute
					}

					return `URI="` + proxyURL + `"`
				},
			)

			if rewriteErr != nil {
				return "", rewriteErr
			}

			continue
		}

		proxyURL, err := registerDirectHLSURL(
			channelID,
			base,
			trimmed,
		)
		if err != nil {
			return "", err
		}

		lines[i] = proxyURL
	}

	return strings.Join(lines, "\n"), nil
}

func registerDirectHLSURL(
	channelID string,
	base *url.URL,
	reference string,
) (string, error) {
	ref, err := url.Parse(reference)
	if err != nil {
		return "", err
	}

	resolved := base.ResolveReference(ref)

	if err := validateDirectHLSURL(resolved.String()); err != nil {
		return "", err
	}

	id, err := registerDirectHLSResource(
		channelID,
		resolved.String(),
	)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"/hls-direct/%s/r-%s",
		channelID,
		id,
	), nil
}
