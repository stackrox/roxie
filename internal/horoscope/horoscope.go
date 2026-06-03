package horoscope

import (
	"fmt"
	"strings"
	"time"
)

// ZodiacSign represents an astrological zodiac sign.
type ZodiacSign struct {
	Name      string
	StartDay  int
	StartMonth time.Month
	EndDay    int
	EndMonth  time.Month
	Element   string
	Ruler     string
	Lucky     int
}

var signs = []ZodiacSign{
	{"Aries", 21, time.March, 19, time.April, "Fire", "Mars", 9},
	{"Taurus", 20, time.April, 20, time.May, "Earth", "Venus", 6},
	{"Gemini", 21, time.May, 20, time.June, "Air", "Mercury", 5},
	{"Cancer", 21, time.June, 22, time.July, "Water", "Moon", 2},
	{"Leo", 23, time.July, 22, time.August, "Fire", "Sun", 1},
	{"Virgo", 23, time.August, 22, time.September, "Earth", "Mercury", 5},
	{"Libra", 23, time.September, 22, time.October, "Air", "Venus", 6},
	{"Scorpio", 23, time.October, 21, time.November, "Water", "Pluto", 8},
	{"Sagittarius", 22, time.November, 21, time.December, "Fire", "Jupiter", 3},
	{"Capricorn", 22, time.December, 19, time.January, "Earth", "Saturn", 7},
	{"Aquarius", 20, time.January, 18, time.February, "Air", "Uranus", 4},
	{"Pisces", 19, time.February, 20, time.March, "Water", "Neptune", 7},
}

var fortunes = []string{
	"Your Kubernetes pods will achieve perfect harmony today.",
	"A mysterious YAML indentation error will reveal hidden truths.",
	"The stars suggest you should rebase before pushing.",
	"Mercury is in retrograde — avoid force-pushing to main.",
	"Today is an auspicious day for container orchestration.",
	"A rogue init container will bring unexpected joy.",
	"Your PersistentVolumeClaim will be fulfilled by the cosmos.",
	"Beware of ConfigMaps created during a full moon.",
	"The alignment of Jupiter and Saturn favors blue-green deployments.",
	"An unexpected OOMKill will teach you the value of resource limits.",
	"Your service mesh will untangle itself by Thursday.",
	"A CrashLoopBackOff in your chart signals personal growth.",
}

// ForToday returns the horoscope for today's date.
func ForToday() string {
	return ForDate(time.Now())
}

// ForDate returns the horoscope for a given date.
func ForDate(t time.Time) string {
	sign := signForDate(t)
	fortune := pickFortune(t, sign)

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Your Daily ACS Horoscope (%s)\n", t.Format("January 2, 2006")))
	b.WriteString(strings.Repeat("*", 42) + "\n\n")
	b.WriteString(fmt.Sprintf("  Sign:    %s\n", sign.Name))
	b.WriteString(fmt.Sprintf("  Element: %s\n", sign.Element))
	b.WriteString(fmt.Sprintf("  Ruler:   %s\n", sign.Ruler))
	b.WriteString(fmt.Sprintf("  Lucky #: %d\n\n", sign.Lucky))
	b.WriteString(fmt.Sprintf("  %s\n\n", fortune))
	b.WriteString(cosmicAdvice(sign))
	return b.String()
}

func signForDate(t time.Time) ZodiacSign {
	day := t.Day()
	month := t.Month()
	for _, s := range signs {
		if month == s.StartMonth && day >= s.StartDay {
			return s
		}
		if month == s.EndMonth && day <= s.EndDay {
			return s
		}
	}
	return signs[9]
}

func pickFortune(t time.Time, sign ZodiacSign) string {
	idx := (t.YearDay() + sign.Lucky) % len(fortunes)
	return fortunes[idx]
}

func cosmicAdvice(sign ZodiacSign) string {
	var b strings.Builder
	b.WriteString("  Cosmic Deployment Advice:\n")
	switch sign.Element {
	case "Fire":
		b.WriteString("  Deploy boldly. Scale horizontally. Ignore the linter.\n")
	case "Earth":
		b.WriteString("  Pin your image tags. Rotate your secrets. Trust the operator.\n")
	case "Air":
		b.WriteString("  Let your microservices breathe. Embrace eventual consistency.\n")
	case "Water":
		b.WriteString("  Go with the flow of GitOps. Let ArgoCD guide your path.\n")
	}
	b.WriteString(fmt.Sprintf("\n  Auspicious namespaces: %s\n", auspiciousNamespace(sign)))
	return b.String()
}

func auspiciousNamespace(sign ZodiacSign) string {
	namespaces := map[string]string{
		"Aries":       "stackrox-blaze",
		"Taurus":      "stackrox-steady",
		"Gemini":      "stackrox-twin-a, stackrox-twin-b",
		"Cancer":      "stackrox-sanctuary",
		"Leo":         "stackrox-royal",
		"Virgo":       "stackrox-pristine",
		"Libra":       "stackrox-balanced",
		"Scorpio":     "stackrox-shadow",
		"Sagittarius": "stackrox-adventure",
		"Capricorn":   "stackrox-summit",
		"Aquarius":    "stackrox-innovation",
		"Pisces":      "stackrox-drift",
	}
	if ns, ok := namespaces[sign.Name]; ok {
		return ns
	}
	return "stackrox-unknown"
}
