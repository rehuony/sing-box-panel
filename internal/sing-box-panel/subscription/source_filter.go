// SPDX-License-Identifier: GPL-3.0-or-later

package subscription

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	maximumSourceFilterRules     = 256
	maximumSourceFilterRuleBytes = 512
)

// SourceFilter selects a source's nodes by name. Plain text matches a
// case-insensitive substring; text wrapped in slashes is an RE2 expression.
// A nonempty include list keeps only matching nodes, and the exclude list
// then removes matching ones.
type SourceFilter struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

// SourceFieldError names a source setting, such as a filter rule, that is
// not valid. Path locates it in the request and Code is fixed.
type SourceFieldError struct {
	Path string
	Code string
}

func (err *SourceFieldError) Error() string {
	return fmt.Sprintf("invalid subscription source: %s: %s", err.Path, err.Code)
}

type nameRule struct {
	text    string
	pattern *regexp.Regexp
}

func (rule nameRule) matches(name string) bool {
	if rule.pattern != nil {
		return rule.pattern.MatchString(name)
	}
	return strings.Contains(strings.ToLower(name), rule.text)
}

type compiledFilter struct {
	include, exclude []nameRule
}

// Validate reports the first rule that is not valid.
func (filter SourceFilter) Validate() error {
	_, err := filter.compile()
	return err
}

func (filter SourceFilter) compile() (compiledFilter, error) {
	compile := func(rules []string, path string) ([]nameRule, error) {
		if len(rules) > maximumSourceFilterRules {
			return nil, &SourceFieldError{Path: path, Code: "too_many_rules"}
		}
		compiled := make([]nameRule, 0, len(rules))
		for index, rule := range rules {
			rulePath := fmt.Sprintf("%s[%d]", path, index)
			if rule == "" || len(rule) > maximumSourceFilterRuleBytes {
				return nil, &SourceFieldError{Path: rulePath, Code: "invalid_length"}
			}
			if len(rule) >= 2 && strings.HasPrefix(rule, "/") && strings.HasSuffix(rule, "/") {
				pattern, err := regexp.Compile(rule[1 : len(rule)-1])
				if err != nil {
					return nil, &SourceFieldError{Path: rulePath, Code: "invalid_regular_expression"}
				}
				compiled = append(compiled, nameRule{pattern: pattern})
				continue
			}
			compiled = append(compiled, nameRule{text: strings.ToLower(rule)})
		}
		return compiled, nil
	}
	include, err := compile(filter.Include, "filter.include")
	if err != nil {
		return compiledFilter{}, err
	}
	exclude, err := compile(filter.Exclude, "filter.exclude")
	if err != nil {
		return compiledFilter{}, err
	}
	return compiledFilter{include: include, exclude: exclude}, nil
}

func (filter compiledFilter) keep(name string) bool {
	matches := func(rules []nameRule) bool {
		for _, rule := range rules {
			if rule.matches(name) {
				return true
			}
		}
		return false
	}
	if len(filter.include) > 0 && !matches(filter.include) {
		return false
	}
	return !matches(filter.exclude)
}
