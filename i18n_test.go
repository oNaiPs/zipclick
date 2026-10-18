package main

import (
	"reflect"
	"regexp"
	"testing"
	"time"
)

var verb = regexp.MustCompile(`%[sd]`)

// Every language has every string, with the same %s/%d verbs in the same
// order as English: a missing or extra verb breaks the Sprintf at run time.
func TestCatalogIsComplete(t *testing.T) {
	ref := reflect.ValueOf(en)
	for lang, m := range catalog {
		v := reflect.ValueOf(*m)
		for i := 0; i < v.NumField(); i++ {
			name := v.Type().Field(i).Name
			f := v.Field(i)
			if f.IsZero() {
				t.Errorf("%s: %s is empty", lang, name)
				continue
			}
			if f.Kind() != reflect.String {
				continue
			}
			got, want := verb.FindAllString(f.String(), -1), verb.FindAllString(ref.Field(i).String(), -1)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s: %s has verbs %v, English has %v", lang, name, got, want)
			}
		}
	}
}

func TestPickLanguage(t *testing.T) {
	for _, c := range []struct {
		prefs []string
		want  string
	}{
		{nil, "en"},
		{[]string{"pt-PT"}, "pt-PT"},
		{[]string{"pt-BR", "en-US"}, "pt-BR"},
		{[]string{"pt-AO"}, "pt-PT"},
		{[]string{"pt"}, "pt-PT"},
		{[]string{"de-AT"}, "de"},
		{[]string{"fr_CA"}, "fr"},
		{[]string{"ja-JP", "es-MX"}, "es"},
		{[]string{"ja-JP"}, "en"},
		{[]string{"EN-gb"}, "en"},
	} {
		if got := pickLanguage(c.prefs); got != c.want {
			t.Errorf("pickLanguage(%v) = %s, want %s", c.prefs, got, c.want)
		}
	}
}

func TestDetailTextIsLocalised(t *testing.T) {
	defer func() { msg = &en }()
	msg = &ptPT
	if got, want := detailText(10<<20, 40<<20, 10*time.Second), "10,0 MB de 40,0 MB, faltam cerca de 30 segundos"; got != want {
		t.Errorf("pt-PT: %q, want %q", got, want)
	}
	msg = &fr
	if got, want := detailText(512, 1<<30, time.Second), "512 octets sur 1,0 Go"; got != want {
		t.Errorf("fr: %q, want %q", got, want)
	}
}

// No em or en dashes, and no arrows, in anything a person reads: they are
// the tell of generated text, and Windows' own docs write menu paths with ">".
func TestCopyHasNoDashesOrArrows(t *testing.T) {
	for lang, m := range catalog {
		v := reflect.ValueOf(*m)
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.Kind() == reflect.String && regexp.MustCompile("[—–→]").MatchString(f.String()) {
				t.Errorf("%s: %s: %q", lang, v.Type().Field(i).Name, f.String())
			}
		}
	}
}
