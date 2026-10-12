package main

import (
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// skillMarker tells unwrap a skill file is gascoigne's.
const skillMarker = "<!-- gascoigne:skill -->"

// claudeSkills are the commands install adds to claude: typed as /gascoigne-stats.
// disable-model-invocation keeps their descriptions out of every request.
var claudeSkills = map[string]string{
	"gascoigne-stats": "---\nname: gascoigne-stats\ndescription: Show what gascoigne saved in recent sessions\ndisable-model-invocation: true\n---\n" + skillMarker +
		"\nRun `gascoigne gain -since 1d` and `gascoigne gain -by command -since 1d`. Report the totals and the three commands with the most tokens saved, briefly.\n",
	"gascoigne-doctor": "---\nname: gascoigne-doctor\ndescription: Check that gascoigne is set up for this agent\ndisable-model-invocation: true\n---\n" + skillMarker +
		"\nRun `gascoigne doctor`. Report each problem with its fix line; if there are none, say so in one line.\n",
}

func claudeSkillFile(global bool, name string) (string, error) {
	rel := filepath.Join(".claude", "skills", name, "SKILL.md")
	return scoped(global, rel, rel)
}

// addClaudeSkills writes the skills, replacing ours and leaving a file of
// the same name that the user wrote.
func addClaudeSkills(global bool) ([]string, error) {
	var done []string
	var errs []error
	for _, name := range slices.Sorted(maps.Keys(claudeSkills)) {
		file, err := claudeSkillFile(global, name)
		if err != nil {
			return done, err
		}
		_, err = writeConfig(file, func(text string) (string, error) {
			if text != "" && !strings.Contains(text, skillMarker) {
				return "", errors.New("exists and was not written by gascoigne, left alone")
			}
			return claudeSkills[name], nil
		})
		if errors.Is(err, errUnchanged) {
			continue
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		done = append(done, display(file)+": wrote /"+name)
	}
	return done, errors.Join(errs...)
}

// removeClaudeSkills deletes gascoigne's skills from the user and project dirs.
func removeClaudeSkills() ([]string, error) {
	var done []string
	var errs []error
	for _, global := range []bool{true, false} {
		for _, name := range slices.Sorted(maps.Keys(claudeSkills)) {
			file, err := claudeSkillFile(global, name)
			if err != nil {
				continue
			}
			data, err := os.ReadFile(file)
			if err != nil || !strings.Contains(string(data), skillMarker) {
				continue
			}
			if err := os.Remove(file); err != nil {
				errs = append(errs, err)
				continue
			}
			os.Remove(filepath.Dir(file)) // only when empty
			done = append(done, display(file)+": deleted /"+name)
		}
	}
	return done, errors.Join(errs...)
}
