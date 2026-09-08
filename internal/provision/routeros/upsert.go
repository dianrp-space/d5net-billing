package routeros

import (
	"strings"

	"github.com/go-routeros/routeros/v3"
)

func parseROSProp(p string) (key, val string, ok bool) {
	p = strings.TrimPrefix(strings.TrimSpace(p), "=")
	i := strings.IndexByte(p, '=')
	if i <= 0 {
		return "", "", false
	}
	key = strings.TrimSpace(p[:i])
	val = p[i+1:]
	if key == "" || key == "numbers" || key == ".id" {
		return "", "", false
	}
	return key, val, true
}

func isROSTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "true", "1":
		return true
	default:
		return false
	}
}

func rosValuesEqual(key, want, have string) bool {
	want = strings.TrimSpace(want)
	have = strings.TrimSpace(have)
	switch key {
	case "enabled":
		return isROSTrue(want) == isROSTrue(have)
	case "disabled":
		wantOff := want == "" || want == "no" || want == "false"
		haveOff := have == "" || have == "no" || have == "false"
		return wantOff == haveOff
	default:
		return want == have
	}
}

// rowMatchesProps reports whether a RouterOS print row already has the desired properties.
func rowMatchesProps(row map[string]string, props []string) bool {
	if row == nil {
		return false
	}
	for _, p := range props {
		key, want, ok := parseROSProp(p)
		if !ok {
			continue
		}
		if !rosValuesEqual(key, want, row[key]) {
			return false
		}
	}
	return true
}

func fieldFilled(row map[string]string, field string) bool {
	return strings.TrimSpace(row[field]) != ""
}

// upsertByName sets properties on an existing named row, or adds it if missing.
// Skips /set when the row already matches (avoids MikroTik system log spam).
func upsertByName(cl *routeros.Client, basePath, name string, props []string) (*routeros.Reply, error) {
	reply, err := cl.Run(basePath+"/print", "?name="+name)
	if err == nil && reply != nil {
		for _, re := range reply.Re {
			id := re.Map[".id"]
			if id == "" {
				continue
			}
			if rowMatchesProps(re.Map, props) {
				return reply, nil
			}
			args := append([]string{basePath + "/set", "=numbers=" + id}, props...)
			return cl.Run(args...)
		}
	}
	args := append([]string{basePath + "/add", "=name=" + name}, props...)
	return cl.Run(args...)
}

func upsertByComment(cl *routeros.Client, basePath, comment string, props []string) error {
	reply, err := cl.Run(basePath + "/print")
	if err == nil && reply != nil {
		for _, re := range reply.Re {
			id := re.Map[".id"]
			cmt := re.Map["comment"]
			if id == "" || strings.TrimSpace(cmt) != strings.TrimSpace(comment) {
				continue
			}
			if rowMatchesProps(re.Map, props) {
				return nil
			}
			args := append([]string{basePath + "/set", "=numbers=" + id}, props...)
			_, err = cl.Run(args...)
			return err
		}
	}
	args := append([]string{basePath + "/add"}, props...)
	_, err = cl.Run(args...)
	return err
}

func ensureProxyEnabled(cl *routeros.Client, port string) error {
	if port == "" {
		port = isolirDefaultProxyPort
	}
	reply, err := cl.Run("/ip/proxy/print")
	if err == nil && reply != nil && len(reply.Re) > 0 {
		m := reply.Re[0].Map
		havePort := strings.TrimSpace(m["port"])
		if isROSTrue(m["enabled"]) && (havePort == port || havePort == "") {
			return nil
		}
	}
	_, err = cl.Run("/ip/proxy/set", "=enabled=yes", "=port="+port)
	return err
}
