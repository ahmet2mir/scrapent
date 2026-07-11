package scrapent

import "testing"

func TestSafeName(t *testing.T) {
	cases := map[string]string{
		"La rentrée des CP/CE1": "la-rentree-des-cpce1",
		"Sortie à la piscine 🏊": "sortie-a-la-piscine",
		"  Noël 2025 !!!  ":     "noel-2025",
		"Fête de l'école (été)": "fete-de-lecole-ete",
		"---":                   "untitled",
		"🎉😀":                    "untitled",
		"Cœur & âme":            "coeur-ame",
		"Multiple   spaces":     "multiple-spaces",
		"ÉLÈVES":                "eleves",
	}
	for in, want := range cases {
		if got := SafeName(in); got != want {
			t.Errorf("SafeName(%q) = %q, want %q", in, got, want)
		}
	}
}
