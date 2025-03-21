package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	router "github.com/v2fly/v2ray-core/v5/app/router/routercommon"
)

// ListInfo is the information structure of a single file in data directory.
// It includes all types of rules of the file, as well as servel types of
// sturctures of same items for convenience in later process.
type ListInfo struct {
	Name                    fileName
	HasInclusion            bool
	InclusionAttributeMap   map[fileName][]attribute
	FullTypeList            []*router.Domain
	KeywordTypeList         []*router.Domain
	RegexpTypeList          []*router.Domain
	AttributeRuleUniqueList []*router.Domain
	DomainTypeList          []*router.Domain
	DomainTypeUniqueList    []*router.Domain
	AttributeRuleListMap    map[attribute][]*router.Domain
	GeoSite                 *router.GeoSite
}

// NewListInfo return a ListInfo
func NewListInfo() *ListInfo {
	return &ListInfo{
		InclusionAttributeMap:   make(map[fileName][]attribute),
		FullTypeList:            make([]*router.Domain, 0, 10),
		KeywordTypeList:         make([]*router.Domain, 0, 10),
		RegexpTypeList:          make([]*router.Domain, 0, 10),
		AttributeRuleUniqueList: make([]*router.Domain, 0, 10),
		DomainTypeList:          make([]*router.Domain, 0, 10),
		DomainTypeUniqueList:    make([]*router.Domain, 0, 10),
		AttributeRuleListMap:    make(map[attribute][]*router.Domain),
	}
}

// ProcessList processes each line of every single file in the data directory
// and generates a ListInfo of each file.
func (l *ListInfo) ProcessList(file *os.File) error {
	scanner := bufio.NewScanner(file)
	// Parse a file line by line to generate ListInfo
	for scanner.Scan() {
		line := scanner.Text()
		if isEmpty(line) {
			continue
		}
		line = removeComment(line)
		if isEmpty(line) {
			continue
		}
		parsedRule, err := l.parseRule(line)
		if err != nil {
			return err
		}
		if parsedRule == nil {
			continue
		}
		l.classifyRule(parsedRule)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}

// parseRule parses a single rule
func (l *ListInfo) parseRule(line string) (*router.Domain, error) {
	line = strings.TrimSpace(line)

	if line == "" {
		return nil, errors.New("empty line")
	}

	// Parse `include` rule first, eg: `include:google`, `include:google @cn @gfw`
	if strings.HasPrefix(line, "include:") {
		l.parseInclusion(line)
		return nil, nil
	}

	parts := strings.Split(line, " ")
	ruleWithType := strings.TrimSpace(parts[0])
	if ruleWithType == "" {
		return nil, errors.New("empty rule")
	}

	var rule router.Domain
	if err := l.parseTypeRule(ruleWithType, &rule); err != nil {
		return nil, err
	}

	for _, attrString := range parts[1:] {
		if attrString = strings.TrimSpace(attrString); attrString != "" {
			attr, err := l.parseAttribute(attrString)
			if err != nil {
				return nil, err
			}
			rule.Attribute = append(rule.Attribute, attr)
		}
	}

	return &rule, nil
}

func (l *ListInfo) parseInclusion(inclusion string) {
	inclusionVal := strings.TrimPrefix(strings.TrimSpace(inclusion), "include:")
	l.HasInclusion = true
	inclusionValSlice := strings.Split(inclusionVal, "@")
	filename := fileName(strings.ToUpper(strings.TrimSpace(inclusionValSlice[0])))
	switch len(inclusionValSlice) {
	case 1: // Inclusion without attribute
		// Use '@' as the placeholder attribute for 'include:filename'
		l.InclusionAttributeMap[filename] = append(l.InclusionAttributeMap[filename], attribute("@"))
	default: // Inclusion with attribute(s)
		// support new inclusion syntax, eg: `include:google @cn @gfw`
		for _, attr := range inclusionValSlice[1:] {
			attr = strings.ToLower(strings.TrimSpace(attr))
			if attr != "" {
				// Added in this format: '@cn'
				l.InclusionAttributeMap[filename] = append(l.InclusionAttributeMap[filename], attribute("@"+attr))
			}
		}
	}
}

func (l *ListInfo) parseTypeRule(domain string, rule *router.Domain) error {
	kv := strings.Split(domain, ":")
	switch len(kv) {
	case 1: // line without type prefix
		rule.Type = router.Domain_RootDomain
		rule.Value = strings.ToLower(strings.TrimSpace(kv[0]))
	case 2: // line with type prefix
		ruleType := strings.TrimSpace(kv[0])
		ruleVal := strings.TrimSpace(kv[1])
		rule.Value = strings.ToLower(ruleVal)
		switch strings.ToLower(ruleType) {
		case "full":
			rule.Type = router.Domain_Full
		case "domain":
			rule.Type = router.Domain_RootDomain
		case "keyword":
			rule.Type = router.Domain_Plain
		case "regexp":
			rule.Type = router.Domain_Regex
			rule.Value = ruleVal
		default:
			return errors.New("unknown domain type: " + ruleType)
		}
	}
	return nil
}

func (l *ListInfo) parseAttribute(attr string) (*router.Domain_Attribute, error) {
	if attr[0] != '@' {
		return nil, errors.New("invalid attribute: " + attr)
	}
	attr = attr[1:] // Trim out attribute prefix `@` character

	var attribute router.Domain_Attribute
	attribute.Key = strings.ToLower(attr)
	attribute.TypedValue = &router.Domain_Attribute_BoolValue{BoolValue: true}
	return &attribute, nil
}

// classifyRule classifies a single rule and write into *ListInfo
func (l *ListInfo) classifyRule(rule *router.Domain) {
	if len(rule.Attribute) > 0 {
		l.AttributeRuleUniqueList = append(l.AttributeRuleUniqueList, rule)
		var attrsString attribute
		for _, attr := range rule.Attribute {
			attrsString += attribute("@" + attr.GetKey()) // attrsString will be "@cn@ads" if there are more than one attributes
		}
		l.AttributeRuleListMap[attrsString] = append(l.AttributeRuleListMap[attrsString], rule)
	} else {
		switch rule.Type {
		case router.Domain_Full:
			l.FullTypeList = append(l.FullTypeList, rule)
		case router.Domain_RootDomain:
			l.DomainTypeList = append(l.DomainTypeList, rule)
		case router.Domain_Plain:
			l.KeywordTypeList = append(l.KeywordTypeList, rule)
		case router.Domain_Regex:
			l.RegexpTypeList = append(l.RegexpTypeList, rule)
		}
	}
}

// Flatten flattens the rules in a file that have "include" syntax
// in data directory, and adds those need-to-included rules into it.
// This feature supports the "include:filename@attribute" syntax.
// It also generates a domain trie of domain-typed rules for each file
// to remove duplications of them.
func (l *ListInfo) Flatten(lm *ListInfoMap) error {
	if l.HasInclusion {
		for filename, attrs := range l.InclusionAttributeMap {
			for _, attrWanted := range attrs {
				includedList := (*lm)[filename]
				switch string(attrWanted) {
				case "@":
					l.FullTypeList = append(l.FullTypeList, includedList.FullTypeList...)
					l.DomainTypeList = append(l.DomainTypeList, includedList.DomainTypeList...)
					l.KeywordTypeList = append(l.KeywordTypeList, includedList.KeywordTypeList...)
					l.RegexpTypeList = append(l.RegexpTypeList, includedList.RegexpTypeList...)
					l.AttributeRuleUniqueList = append(l.AttributeRuleUniqueList, includedList.AttributeRuleUniqueList...)
					for attr, domainList := range includedList.AttributeRuleListMap {
						l.AttributeRuleListMap[attr] = append(l.AttributeRuleListMap[attr], domainList...)
					}

				default:
					for attr, domainList := range includedList.AttributeRuleListMap {
						// If there are more than one attribute attached to the rule,
						// the attribute key of AttributeRuleListMap in ListInfo
						// will be like: "@cn@ads".
						// So if to extract rules with a specific attribute, it is necessary
						// also to test the multi-attribute keys of AttributeRuleListMap.
						// Notice: if "include:google @cn" and "include:google @ads" appear
						// at the same time in the parent list. There are chances that the same
						// rule with that two attributes(`@cn` and `@ads`) will be included twice in the parent list.
						if strings.Contains(string(attr)+"@", string(attrWanted)+"@") {
							l.AttributeRuleListMap[attr] = append(l.AttributeRuleListMap[attr], domainList...)
							l.AttributeRuleUniqueList = append(l.AttributeRuleUniqueList, domainList...)
						}
					}
				}
			}
		}
	}

	sort.Slice(l.DomainTypeList, func(i, j int) bool {
		return len(strings.Split(l.DomainTypeList[i].GetValue(), ".")) < len(strings.Split(l.DomainTypeList[j].GetValue(), "."))
	})

	trie := NewDomainTrie()
	for _, domain := range l.DomainTypeList {
		success, err := trie.Insert(domain.GetValue())
		if err != nil {
			return err
		}
		if success {
			l.DomainTypeUniqueList = append(l.DomainTypeUniqueList, domain)
		}
	}

	return nil
}

// ToGeoSite converts every ListInfo into a router.GeoSite structure.
// It also excludes rules with certain attributes in certain files that
// user specified in command line when runing the program.
func (l *ListInfo) ToGeoSite(excludeAttrs map[fileName]map[attribute]bool) {
	geosite := new(router.GeoSite)
	geosite.CountryCode = string(l.Name)

	// 1. First collect all full domain rules (including those with attributes)
	geosite.Domain = append(geosite.Domain, l.FullTypeList...)
	for _, domain := range l.AttributeRuleUniqueList {
		if domain.Type == router.Domain_Full {
			if excludeAttrs != nil && excludeAttrs[l.Name] != nil {
				excludeAttrsMap := excludeAttrs[l.Name]
				ifKeep := true
				for _, attr := range domain.GetAttribute() {
					if excludeAttrsMap[attribute(attr.GetKey())] {
						ifKeep = false
						break
					}
				}
				if ifKeep {
					geosite.Domain = append(geosite.Domain, domain)
				}
			} else {
				geosite.Domain = append(geosite.Domain, domain)
			}
		}
	}

	// 2. Then add all domain suffix rules (including those with attributes)
	geosite.Domain = append(geosite.Domain, l.DomainTypeUniqueList...)
	for _, domain := range l.AttributeRuleUniqueList {
		if domain.Type == router.Domain_RootDomain {
			if excludeAttrs != nil && excludeAttrs[l.Name] != nil {
				excludeAttrsMap := excludeAttrs[l.Name]
				ifKeep := true
				for _, attr := range domain.GetAttribute() {
					if excludeAttrsMap[attribute(attr.GetKey())] {
						ifKeep = false
						break
					}
				}
				if ifKeep {
					geosite.Domain = append(geosite.Domain, domain)
				}
			} else {
				geosite.Domain = append(geosite.Domain, domain)
			}
		}
	}

	l.GeoSite = geosite
}

// ToPlainText convert router.GeoSite structure to plaintext format.
func (l *ListInfo) ToPlainText() []byte {
	plaintextBytes := make([]byte, 0, 1024*512)

	// Add header comments
	plaintextBytes = append(plaintextBytes, []byte("# Generated by https://github.com/caocaocc/rule-set\n")...)
	plaintextBytes = append(plaintextBytes, []byte("# Last Modified: " + time.Now().Format(time.RFC1123) + "\n\n")...)

	for _, rule := range l.GeoSite.Domain {
		ruleVal := strings.TrimSpace(rule.GetValue())
		if len(ruleVal) == 0 {
			continue
		}

		var ruleString string
		switch rule.Type {
		case router.Domain_Full:
			ruleString = "full:" + ruleVal
		case router.Domain_RootDomain:
			ruleString = "domain:" + ruleVal
		case router.Domain_Plain:
			ruleString = "keyword:" + ruleVal
		case router.Domain_Regex:
			ruleString = "regexp:" + ruleVal
		}

		if len(rule.Attribute) > 0 {
			ruleString += ":"
			for _, attr := range rule.Attribute {
				ruleString += "@" + attr.GetKey() + ","
			}
			ruleString = strings.TrimRight(ruleString, ",")
		}
		// Output format is: type:domain.tld:@attr1,@attr2
		plaintextBytes = append(plaintextBytes, []byte(ruleString+"\n")...)
	}

	return plaintextBytes
}

// ToGFWList converts router.GeoSite to GFWList format.
func (l *ListInfo) ToGFWList() []byte {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	timeString := fmt.Sprintf("! Last Modified: %s\n", time.Now().In(loc).Format(time.RFC1123))

	gfwlistBytes := make([]byte, 0, 1024*512)
	gfwlistBytes = append(gfwlistBytes, []byte("[AutoProxy 0.2.9]\n")...)
	gfwlistBytes = append(gfwlistBytes, []byte(timeString)...)
	gfwlistBytes = append(gfwlistBytes, []byte("! Expires: 24h\n")...)
	gfwlistBytes = append(gfwlistBytes, []byte("! HomePage: https://github.com/caocaocc/rule-set\n")...)
	gfwlistBytes = append(gfwlistBytes, []byte("! GitHub URL: https://raw.githubusercontent.com/caocaocc/rule-set/release/gfwlist.txt\n")...)
	gfwlistBytes = append(gfwlistBytes, []byte("! jsdelivr URL: https://cdn.jsdelivr.net/gh/caocaocc/rule-set@release/gfwlist.txt\n")...)
	gfwlistBytes = append(gfwlistBytes, []byte("\n")...)

	for _, rule := range l.GeoSite.Domain {
		ruleVal := strings.TrimSpace(rule.GetValue())
		if len(ruleVal) == 0 {
			continue
		}

		switch rule.Type {
		case router.Domain_Full:
			gfwlistBytes = append(gfwlistBytes, []byte("|http://"+ruleVal+"\n")...)
			gfwlistBytes = append(gfwlistBytes, []byte("|https://"+ruleVal+"\n")...)
		case router.Domain_RootDomain:
			gfwlistBytes = append(gfwlistBytes, []byte("||"+ruleVal+"\n")...)
		case router.Domain_Plain:
			gfwlistBytes = append(gfwlistBytes, []byte(ruleVal+"\n")...)
		case router.Domain_Regex:
			gfwlistBytes = append(gfwlistBytes, []byte("/"+ruleVal+"/\n")...)
		}
	}

	return gfwlistBytes
}

// ToSurgeList converts router.GeoSite to Surge rule list format
func (l *ListInfo) ToSurgeList() []byte {
	surgeBytes := make([]byte, 0, 1024*512)
	
	// Add header comments
	surgeBytes = append(surgeBytes, []byte("# Generated by https://github.com/caocaocc/rule-set\n")...)
	surgeBytes = append(surgeBytes, []byte("# Last Modified: " + time.Now().Format(time.RFC1123) + "\n\n")...)

	for _, rule := range l.GeoSite.Domain {
		ruleVal := strings.TrimSpace(rule.GetValue())
		if len(ruleVal) == 0 {
			continue
		}

		// Convert different rule types to Surge format
		switch rule.Type {
		case router.Domain_Full:
			surgeBytes = append(surgeBytes, []byte("DOMAIN," + ruleVal + "\n")...)
		case router.Domain_RootDomain:
			surgeBytes = append(surgeBytes, []byte("DOMAIN-SUFFIX," + ruleVal + "\n")...)
		}
	}

	return surgeBytes
}

// ToMihomoList converts router.GeoSite to Mihomo/Clash.Meta YAML format
func (l *ListInfo) ToMihomoList() []byte {
	yamlBytes := make([]byte, 0, 1024*512)
	
	// Add header comments and payload
	yamlBytes = append(yamlBytes, []byte("# Generated by https://github.com/caocaocc/rule-set\n")...)
	yamlBytes = append(yamlBytes, []byte("# Last Modified: " + time.Now().Format(time.RFC1123) + "\n\n")...)
	yamlBytes = append(yamlBytes, []byte("payload:\n")...)

	for _, rule := range l.GeoSite.Domain {
		ruleVal := strings.TrimSpace(rule.GetValue())
		if len(ruleVal) == 0 {
			continue
		}

		// Convert different rule types to Mihomo/Clash.Meta format
		switch rule.Type {
		case router.Domain_Full:
			// Full domain match should use exact domain
			yamlBytes = append(yamlBytes, []byte("  - '" + ruleVal + "'\n")...)
		case router.Domain_RootDomain:
			// Root domain should use +. prefix which matches the domain itself and all subdomains
			yamlBytes = append(yamlBytes, []byte("  - '+." + ruleVal + "'\n")...)
		}
	}

	return yamlBytes
}

// ToSingBoxList converts router.GeoSite to sing-box rule list format
func (l *ListInfo) ToSingBoxList() []byte {
	type DomainRule struct {
		Domain        []string `json:"domain,omitempty"`
		DomainSuffix []string `json:"domain_suffix,omitempty"`
	}

	type SingBoxRuleSet struct {
		Version int          `json:"version"`
		Rules   []DomainRule `json:"rules"`
	}

	// Create rule set with single rule
	ruleSet := SingBoxRuleSet{
		Version: 2,
		Rules: []DomainRule{
			{
				Domain:        make([]string, 0, 1024),
				DomainSuffix: make([]string, 0, 1024),
			},
		},
	}

	// Process rules in original order
	for _, rule := range l.GeoSite.Domain {
		ruleVal := strings.TrimSpace(rule.GetValue())
		if len(ruleVal) == 0 {
			continue
		}

		switch rule.Type {
		case router.Domain_Full:
			ruleSet.Rules[0].Domain = append(ruleSet.Rules[0].Domain, ruleVal)
		case router.Domain_RootDomain:
			ruleSet.Rules[0].DomainSuffix = append(ruleSet.Rules[0].DomainSuffix, "."+ruleVal)
		}
	}

	jsonBytes, err := json.MarshalIndent(ruleSet, "", "  ")
	if err != nil {
		return nil
	}

	return jsonBytes
}

// ToQuantumultXList converts router.GeoSite to Quantumult X snippet format
func (l *ListInfo) ToQuantumultXList() []byte {
	qxBytes := make([]byte, 0, 1024*512)
	
	// Add header comments
	qxBytes = append(qxBytes, []byte("# Generated by https://github.com/caocaocc/rule-set\n")...)
	qxBytes = append(qxBytes, []byte("# Last Modified: " + time.Now().Format(time.RFC1123) + "\n\n")...)

	// Determine policy based on list name
	policy := "proxy"
	switch l.Name {
	case "PRIVATE", "CN", "TLD-CN", "GEOLOCATION-CN", "BILIBILI":
		policy = "direct"
	}

	for _, rule := range l.GeoSite.Domain {
		ruleVal := strings.TrimSpace(rule.GetValue())
		if len(ruleVal) == 0 {
			continue
		}

		// Convert different rule types to Quantumult X format
		switch rule.Type {
		case router.Domain_Full:
			qxBytes = append(qxBytes, []byte("host, " + ruleVal + ", " + policy + "\n")...)
		case router.Domain_RootDomain:
			qxBytes = append(qxBytes, []byte("host-suffix, " + ruleVal + ", " + policy + "\n")...)
		}
	}

	return qxBytes
}
