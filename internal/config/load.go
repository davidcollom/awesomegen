package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Load(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, err
	}
	if c.User == "" {
		return Config{}, fmt.Errorf("user is required")
	}
	if len(c.Lists) == 0 {
		return Config{}, fmt.Errorf("lists is required")
	}
	for i := range c.Lists {
		if len(c.Lists[i].Sources) == 0 {
			// legacy fallback requires user+slug
			if c.User == "" || c.Lists[i].Slug == "" {
				return Config{}, fmt.Errorf("either lists[%d].sources or {user + slug} must be set", i)
			}
			c.Lists[i].Sources = []Source{{User: c.User, Slug: c.Lists[i].Slug}}
		}
		if c.Lists[i].TopicFallback == "" {
			c.Lists[i].TopicFallback = "misc"
		}
		if c.Lists[i].TopicGroupingMode == "" {
			c.Lists[i].TopicGroupingMode = "flat"
		}
		if c.Lists[i].StarsFormat == "" {
			c.Lists[i].StarsFormat = "locale"
		}
		if c.Lists[i].Locale == "" {
			c.Lists[i].Locale = "en-GB"
		}
		if c.Lists[i].StaleMonths == 0 {
			c.Lists[i].StaleMonths = 24
		}
		if c.Lists[i].TopicFallback == "" {
			c.Lists[i].TopicFallback = "misc"
		}
		if c.Lists[i].TopicGroupingMode == "" {
			c.Lists[i].TopicGroupingMode = "flat"
		}
		if c.Lists[i].StarsFormat == "" {
			c.Lists[i].StarsFormat = "locale"
		}
		if c.Lists[i].Locale == "" {
			c.Lists[i].Locale = "en-GB"
		}
		if c.Lists[i].TopTagsLimit == 0 {
			c.Lists[i].TopTagsLimit = 10
		}
		if c.Lists[i].MinTagCount == 0 {
			c.Lists[i].MinTagCount = 1
		}
		if !c.Lists[i].SingleHome {
			c.Lists[i].SingleHome = true
		} // default true
	}

	return c, nil
}
