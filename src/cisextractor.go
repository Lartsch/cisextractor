package main

import (
	"fmt"
	"path/filepath"
	"time"

	"code.sajari.com/docconv"

	"encoding/csv"
	"os"
	"strings"

	// we use both, regexp2 and regexp for different cases, more below
	"regexp"

	"github.com/dlclark/regexp2"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v3"
)

//  describes location of a rule in a CIS benchmark
type Location struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

// stores section data
type namedValue struct {
	name  string
	value string
}

// Rule describes a CIS benchmark rule
type Rule struct {
	ID        string            `yaml:"id"`
	Name      string            `yaml:"name"`
	Automated bool              `yaml:"automated"`
	Location  []Location        `yaml:"location,omitempty"`
	Sections  map[string]string `yaml:"-,inline"`
}

// start arguments
var (
	inFile     = kingpin.Flag("in", "Filepath to parse (PDF)").Short('i').Required().String()
	outFile    = kingpin.Flag("out", "Optional: Output to filepath (default: ./<pdfname>.<csv|yaml>)").Short('o').String()
	trimBreaks = kingpin.Flag("trimsections", "Optional: Remove all new line characters from section content (default=disabled)").Short('t').Bool()
	usecsv     = kingpin.Flag("csv", "Optional: Export in CSV format (default=YAML)").Short('c').Bool()
	detailed   = kingpin.Flag("details", "Optional: Shows details for identification errors if needed (default=disabled)").Short('d').Bool()
)

// regex strings (we use regexp2 for some tasks since it support lookaheads/lookbehinds - yet regexp has more useful functions)
var (
	// used to remove page markers
	pageMarkerRegex = regexp.MustCompile(`([\d]+\s?\|\s?(Page|P a g e|P age|P a ge|Pa g e|Pag e))`)
	// used to extract titles from the ToC
	titleExtractRegex = regexp2.MustCompile(`(?<=\n)((\d+\.)*?(\d+)+)\s{1}(?:(?:.|\n|\r)*?)\s?\d+((?=\n\d)|(?=\n\n))`, 0)
	// used to clean titles extracted from ToC
	titleCropRegex  = regexp2.MustCompile(`(?<![1-9])\s?[.]+\s?[\d]+$`, 0)
	titleCropRegex2 = regexp.MustCompile(`^(.+?)(?:\s?[\d]+$)?$`)
	// used to split titles into ID and Name by submatches
	titleIDRegex = regexp.MustCompile(`^(\d{1,3}\.?(?:\d{1,3}\.?)*)+((.+?)*)$`)
	// general purpose
	whitespace = regexp.MustCompile(`\s+`)
	// used in section extraction algorithm
	sectionRegex = regexp.MustCompile(`((Profile Applicability|Description|Rationale|Audit|Remediation|Impact|Default\sValue|References|CIS\sControls)\:\s+)`)
	// extract titles from the actual content (differs from ToC)
	ruleTitleExtractRegex = regexp2.MustCompile(`(?<=\n\n)((\d+\.)*?(\d+)+) (.*(\n.*){0,3}|.*\n)(\(Automated|Manual|Scored|Not Scored)\){1}(?=\n(?![.]))`, 0)
	// used to clean some borderline cases for the titles extracted from the actual content
	ruleTitleCleanupRegex = regexp.MustCompile(`(?m)^((\d+\.)*?(\d+)+) `)
	// cleanup non ascii chars
	nonASCIIRegex = regexp.MustCompile(`[[:^ascii:]]`)
	// used to reduce multiple line breaks in content sections
	reduceLinebreakRegex = regexp.MustCompile(`(\r\n?|\n){3,}`)
)

// bridge function to imitate regexp's FindAllString method for regexp2
func regexp2FindAllString(re *regexp2.Regexp, s string) []string {
	// stores the matches
	var matches []string
	// get first match
	m, _ := re.FindStringMatch(s)
	for m != nil {
		// append the match and get the next as long as m != nil
		matches = append(matches, m.String())
		m, _ = re.FindNextMatch(m)
	}
	// cleanup if we have a duplicate
	if matches[0] == matches[len(matches)-1] {
		matches = matches[:len(matches)-1]
	}
	return matches
}

// replace whitespace/linebreaks in regex fashion
func replaceWhitespaces(content string) string {
	replaced := whitespace.ReplaceAllString(content, " ")
	return replaced
}

// remove all page markers from the pdf text in regex fashion
func cutPageMarker(s string) string {
	pm := pageMarkerRegex.ReplaceAllString(s, "")
	return pm
}

// crop the titles for usage
func cropTitles(s []string) []string {
	// for each title...
	for i := 0; i < len(s); i++ {
		// perform the regex cleanup
		s[i], _ = titleCropRegex.Replace(s[i], "", 0, -1)
		// replace windows style linebreaks with normal ones
		s[i] = strings.ReplaceAll(s[i], "\r\n", "\n")
		// replace all new lines
		s[i] = strings.ReplaceAll(s[i], "\n", " ")
		s[i] = titleCropRegex2.FindStringSubmatch(s[i])[1]
	}
	return s
}

// get all titles from the pdf text
func getAllTitlesToC(s string) []string {
	// get all matches for the regex
	titles := regexp2FindAllString(titleExtractRegex, s)
	// we need to fix the last match manually
	if strings.Contains(titles[len(titles)-1], "Appendix:") {
		titles[len(titles)-1] = strings.Split(titles[len(titles)-1], "Appendix:")[0]
	}
	// split it by new lines
	splitted := strings.Split(strings.ReplaceAll(titles[len(titles)-1], "\r\n", "\n"), "\n")
	// if we have more than 1 newline, remove the last split part and rejoin
	if len(splitted) > 1 {
		newvalue := strings.Join(splitted[0:len(splitted)-1], "\n")
		titles[len(titles)-1] = newvalue
	}
	return titles
}

func getAllTitlesContent(content string) []string {
	sectionTitles := regexp2FindAllString(ruleTitleExtractRegex, content)
	// for each identified section title...
	for i := 0; i < len(sectionTitles); i++ {
		// find all match INDEXES for section numbers at start of line
		matches := ruleTitleCleanupRegex.FindAllStringIndex(sectionTitles[i], -1)
		// if we have more than 1 match, we have a borderline case where the parent section title sneaked into our regex
		if len(matches) > 1 {
			// so we remove everything up to the second match, leaving us with only the rule section title
			sectionTitles[i] = sectionTitles[i][matches[1][0]:]
		}
	}
	return sectionTitles
}

// helper function to make HasSuffix work with an array
func hasSuffixAny(s string, suffix []string) bool {
	for _, suf := range suffix {
		if strings.HasSuffix(s, suf) {
			return true
		}
	}
	return false
}

// helper function to remove any suffix (string array) from a string
func removeSuffixAny(s string, suffix []string) string {
	for _, suf := range suffix {
		if strings.HasSuffix(s, suf) {
			return strings.ReplaceAll(s, suf, "")
		}
	}
	return ""
}

// split a title to id, name and determine if it is an actual rule and automated or not
func splitTitle(title string) (id, name string, isActualRule bool, automated bool, err error) {
	// initial values
	isActualRule = false
	automated = false
	// rule types
	rStr := []string{"(Automated)", "(Scored)", "(Manual)", "(Not Scored)"}
	// if it has any of the above strings as suffix, it is a rule
	if hasSuffixAny(title, rStr) {
		isActualRule = true
		// if the suffix is "automated" or "scored", it is an automated rule
		if hasSuffixAny(title, rStr[0:2]) {
			automated = true
		}
		// now remove the suffix for cleanup
		title = removeSuffixAny(title, rStr)
	}
	// get all submatches for our regex
	titleParts := titleIDRegex.FindStringSubmatch(title)
	if titleParts == nil {
		fmt.Printf("failed to split title into id and name: %s", title)
		return
	}
	// part 1 is the id
	id = strings.TrimSpace(replaceWhitespaces(titleParts[1]))
	// part 2 the name
	name = strings.TrimSpace(replaceWhitespaces(titleParts[2]))
	return
}

// returns a Location array, holding each parent chapter up to the chapter of interest
func getRuleLocation(ruleIDToName map[string]string, ruleID string) []Location {
	// stores the locations
	var loc []Location
	// chapters are divided by "."
	const sep = "."
	// split the chapter ID
	parts := strings.Split(ruleID, sep)
	// for each split part...
	for i := 0; i < len(parts)-1; i++ {
		// get the parent ID
		parentID := strings.Join(parts[:i+1], sep)
		// map the parent ID to its name
		if parentName, ok := ruleIDToName[parentID]; ok {
			// append the parent chapter to the loc. array
			loc = append(loc, Location{
				ID:   parentID,
				Name: parentName,
			})
		}
	}
	return loc
}

// used to extract the section content for each rule
func findNamedValuesByRegex(s string, r *regexp.Regexp) []namedValue {
	// get the indexes for all submatches (r will be the section regex, s the content between two rule titles)
	hits := r.FindAllStringSubmatchIndex(s, -1)
	// array to hold results
	var result []namedValue
	// for each matched index...
	for h := 0; h < len(hits); h++ {
		// get start and end of first match
		hit := hits[h]
		// the name of the section is index hit[0] to hit[1] in the string
		name := s[hit[0]:hit[1]]
		// setup empty string for the section content
		var value string
		// if we are not at the last matched index...
		if h != len(hits)-1 {
			// .. then the content will be between hit[1] and hits[h+1][0]
			value = s[hit[1]:hits[h+1][0]]
		} else {
			// if it is the last match, the section content is hit[1] to the end of the string
			value = s[hit[1]:]
		}
		// now append to result array
		result = append(result, namedValue{
			name:  name,
			value: value,
		})
	}
	return result
}

// transform a section name to its key representation (for output)
func sectionKeyName(name string) string {
	key := strings.ToLower(strings.Trim(name, " :\t\n"))
	return whitespace.ReplaceAllString(key, "_")
}

// transform a section content for output
func sectionContent(content string) string {
	content = strings.TrimSpace(nonASCIIRegex.ReplaceAllLiteralString(content, ""))
	content = reduceLinebreakRegex.ReplaceAllString(content, "$1")
	// optional trimming of linebreaks
	if *trimBreaks {
		return replaceWhitespaces(content)
	}
	return content
}

// filtering and preparation of identified titles from the ToC
func prepareRules(titles []string) (noRuleCount int, ruleIDToName map[string]string, noRuleRegexClean *regexp.Regexp, rules []Rule) {
	// this will hold all section titles that are NOT rules, for cleanup purposes later on
	var noRuleString string
	// this holds a map of rule ids to their name, used for building the rule locations later on
	ruleIDToName = map[string]string{}
	for _, title := range titles {
		// for each title from the ToC, get the ID, if it is an rule and if it is automated
		id, name, isActualRule, automated, err := splitTitle(title)
		if err != nil {
			fmt.Println(err)
			continue
		}
		// add the entry to our mapping
		ruleIDToName[id] = name
		// if it is a rule, build the Rule object
		if isActualRule {
			rule := Rule{
				ID:        id,
				Automated: automated,
				Name:      name,
				Sections:  map[string]string{},
			}
			// append it to our Rule array
			rules = append(rules, rule)
		} else {
			// if its not a rule, increase the counter and add it to our "noruleString" (regex, will be used for cleanup later)
			noRuleCount++
			noRuleString = noRuleString + "|" + regexp.QuoteMeta(id) + " " + regexp.QuoteMeta(name)
		}
	}
	// finalize the regex string for "no rule" sections
	noRuleString = strings.TrimPrefix(noRuleString, "|")
	noRuleRegexClean = regexp.MustCompile(`(` + strings.TrimPrefix(noRuleString, "|") + `)[\s\S]*`)
	return noRuleCount, ruleIDToName, noRuleRegexClean, rules
}

// fill the missing contents for our Rule entries
func populateRules(rulesIn []Rule, sectionTitles []string, ruleIDToName map[string]string, content string, noRuleRegexClean *regexp.Regexp) (rulesOut []Rule, ruleErrors []string, sectionErrors []string) {
	// for each rule in our Rule array
	for i := 0; i < len(rulesIn); i++ {
		// grab the ID
		ruleID := rulesIn[i].ID
		// set the location string
		rulesIn[i].Location = getRuleLocation(ruleIDToName, ruleID)
		// setup vars
		var (
			sectionTitle     string
			nextSectionTitle string
		)
		// for each section that we extracted from the actual content...
		for v := 0; v < len(sectionTitles); v++ {
			// if the current rule ID is the prefix for the current sectionTitle from content...
			if strings.HasPrefix(sectionTitles[v], ruleID+" ") {
				// ... set the sectionTitle
				sectionTitle = sectionTitles[v]
			}
			// if we are at the last rule, the next section title will be "Appendix: ..."
			if i == len(rulesIn)-1 {
				nextSectionTitle = "Appendix:"
			} else {
				// otherwise, if the current section title has the NEXT rule ID as prefix...
				if strings.HasPrefix(sectionTitles[v], rulesIn[i+1].ID+" ") {
					// ... set it as the nextSectionTitle
					nextSectionTitle = sectionTitles[v]
				}
			}
		}
		// escape the section titles for regex
		regEscape1 := regexp.QuoteMeta(sectionTitle)
		regEscape2 := regexp.QuoteMeta(nextSectionTitle)
		// build the regex string
		finalRegexString := strings.ReplaceAll(`(?<=`+regEscape1+`\n)(?:(?:.|\n|\r)*?)(?=`+regEscape2+`)`, "\r\n", `\r\n`)
		finalRegexString = strings.ReplaceAll(finalRegexString, "\n", `\n`)
		// compile the regex string
		searchRegex := regexp2.MustCompile(finalRegexString, 0)
		// find the match for our built regex (the match will be the content between the two section titles)
		match, _ := searchRegex.FindStringMatch(content)
		if match == nil {
			sectionErrors = append(sectionErrors, finalRegexString)
		} else {
			// extract sections of rule
			sections := findNamedValuesByRegex(match.String(), sectionRegex)
			if len(sections) == 0 {
				// no content found, so we have an identification issue
				ruleErrors = append(ruleErrors, rulesIn[i].ID)
				continue
			}
			// update the Rule object to include the content sections
			for _, section := range sections {
				rulesIn[i].Sections[sectionKeyName(section.name)] = noRuleRegexClean.ReplaceAllString(sectionContent(section.value), "") // we also cleanup each section content to remove any possible leftovers from non-rule titles
			}
		}
	}
	return rulesIn, ruleErrors, sectionErrors
}

// write either a csv or yaml result file from our Rule array
func writeResultFile(populatedRules []Rule, outFileW string) {
	// CSV mode
	if *usecsv {
		// create the file
		csvFile, err := os.Create(outFileW)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open file %v", err)
			return
		}
		// string array to hold the section types (ordered for output)
		sectionNames := []string{"Profile Applicability", "Description", "Rationale", "Audit", "Remediation", "Impact", "Default Value", "References", "CIS Controls"}
		// holder
		sectionKeyNames := []string{}
		// first columns of the CSV
		csvrecords := [][]string{
			{"ID", "Name", "Location", "Automated"},
		}
		// append the rule sections to our first CSV line (headers)
		for _, section := range sectionNames {
			csvrecords[0] = append(csvrecords[0], section)
			// also append to our key representations of the section names to our array
			sectionKeyNames = append(sectionKeyNames, sectionKeyName(section))
		}
		// now process the actual rules...
		for _, rule := range populatedRules {
			// start with id and name
			csvrecord := []string{rule.ID, rule.Name}
			// build a string from our location array
			locationstr := ""
			for _, location := range rule.Location {
				locationstr = locationstr + location.ID + " " + location.Name + ", "
			}
			locationstr = strings.TrimSuffix(locationstr, ", ")
			// append the location string
			csvrecord = append(csvrecord, locationstr)
			// append the boolean value for automated rule / not automated
			csvrecord = append(csvrecord, fmt.Sprintf("%t", rule.Automated))
			// now go through the section names (ordered)
			for _, sectionKey := range sectionKeyNames {
				// if the section exists, append it
				if val, ok := rule.Sections[sectionKey]; ok {
					csvrecord = append(csvrecord, val)
				} else {
					// otherwise append empty value
					csvrecord = append(csvrecord, " ")
				}
			}
			// finally append the whole csv record
			csvrecords = append(csvrecords, csvrecord)
		}
		fmt.Print(">>> Writing result file...\n\n")
		// write each line of the CSV file...
		w := csv.NewWriter(csvFile)
		for _, record := range csvrecords {
			if err := w.Write(record); err != nil {
				fmt.Println("error writing record to csv:\n", err)
			}
		}
		// Write any buffered data to the underlying writer (standard output).
		w.Flush()
		if err := w.Error(); err != nil {
			fmt.Println(err)
		}
		// close the file
		csvFile.Close()
	} else { // YAML mode
		// stores our output
		var output string
		// marshal rules data to yaml
		yamlData, err := yaml.Marshal(&populatedRules)
		if err != nil {
			fmt.Printf(">>> Failed to serialize as YAML: %v\n\n", err)
			return
		}
		// assign yaml content to our output  var
		output = fmt.Sprintf("---\n%s", string(yamlData))
		f := os.Stdout
		fmt.Print(">>> Writing result file...\n\n")
		// create file
		file, err := os.Create(outFileW)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open file %v", err)
			return
		}
		// write and close the file
		defer file.Close()
		f = file
		fmt.Fprint(f, output)
	}
}

func main() {
	fmt.Printf("\n>>> NOTE: You need the poppler utils installed on your system / put on your $PATH\nPlease refer to the readme file for instructions!\n\n")
	// parse command line arguments
	kingpin.Parse()
	start := time.Now()
	// create copy of outFile argument
	outFileW := *outFile
	// if it is empty, set up the default filename (depends on CSV / YAML mode)
	if outFileW == "" {
		if *usecsv {
			outFileW = strings.TrimSuffix(filepath.Base(*inFile), ".pdf") + "_extracted.csv"
		} else {
			outFileW = strings.TrimSuffix(filepath.Base(*inFile), ".pdf") + "_extracted.yaml"
		}
	}
	// read text from pdf file
	res, err := docconv.ConvertPath(*inFile)
	if err != nil {
		fmt.Println(err)
		// if the file could not be read, EXIT...
		os.Exit(3)
	}
	// remove all page markers
	content := cutPageMarker(res.Body)
	// split to ToC and rest of file content
	splits := strings.Split(content, "Overview")
	toc := splits[1]
	content = splits[2]
	// grab all titles from the ToC
	titles := getAllTitlesToC(toc)
	// cleanup titles (remove dots and trailing site number)
	titles = cropTitles(titles)
	// prepare rules
	noRuleCount, ruleIDToName, noRuleRegexClean, rules := prepareRules(titles)
	// how many actual rules were found?
	ruleCount := len(rules)
	fmt.Printf(">>> Found %d rules (and %d additional parent chapters, total %d) in ToC\n\n", ruleCount, noRuleCount, ruleCount+noRuleCount)
	// we need to grab the actual headings of the text sections separately - they may differ in exact bytes etc. - is used for grabbing the text between two headings
	sectionTitles := getAllTitlesContent(content)
	// give the user a hint on how good the section identification worked
	if len(sectionTitles) == ruleCount {
		fmt.Printf(">>> Count of found text section headings is the same as in table of contents (%d) - great!\n\n", ruleCount)
	} else if len(sectionTitles) > ruleCount {
		fmt.Printf(">>> WARNING: Found more text section headings (%d, %d more) than in ToC - please verify rule contents after extraction.\n\n", len(sectionTitles), len(sectionTitles)-ruleCount)
	} else if len(sectionTitles) < ruleCount {
		fmt.Printf(">>> WARNING: Found less text section headings (%d, %d less) than in ToC - this will cause incomplete data.\n\n", len(sectionTitles), ruleCount-len(sectionTitles))
		// if the -d flag was provided, print the affected rules
		if *detailed {
			// for each rule, check if its ID was found in the section titles extracted from the actual content (not ToC)
			for _, rule := range rules {
				found := false
				for _, section := range sectionTitles {
					if strings.HasPrefix(section, rule.ID+"") {
						found = true
					}
				}
				// if it cant be found, print it
				if !found {
					fmt.Printf("%s\n", rule.ID)
				}
			}
		}
	}
	fmt.Print(">>> Extracting rule locations and content sections...\n\n")
	// now populate our rules with their locations and contents
	populatedRules, ruleErrors, sectionErrors := populateRules(rules, sectionTitles, ruleIDToName, content, noRuleRegexClean)
	// remove the old rules variable (free up RAM)
	rules = nil
	// print any issues for consideration
	fmt.Printf(">>> %d section identification errors, %d rule section identification errors\n\n", len(sectionErrors), len(ruleErrors))
	// if -d flag was provided, print details
	if *detailed {
		if len(ruleErrors) > 0 {
			fmt.Printf(">>> For the following rules no detail sections could be identified in the content analyzed:\n")
			for _, error := range ruleErrors {
				fmt.Printf("%s, ", error)
			}
			fmt.Printf("\n\n")
		}
		if len(sectionErrors) > 0 {
			fmt.Printf(">>> For the following regex expressions no content could be found between the rule titles:\n")
			for _, error := range sectionErrors {
				fmt.Printf("%s\n", error)
			}
			fmt.Println()
		}
	}
	// write the file
	writeResultFile(populatedRules, outFileW)
	fmt.Printf(">>> All done!\n\n\tOutput file:\t%s\n\tTime elapsed:\t%s\n\n", outFileW, time.Since(start))
}
