package stage

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	models "tf200/todoapp/internal"
)

func st(id, name string) models.Stage {
	return models.Stage{ID: id, Name: name, Color: "#F59E0B"}
}

func TestClean(t *testing.T) {
	got, err := Clean([]models.Stage{st("a", "  To show "), {Name: "Testing", Color: "#3b82f6"}})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Name != "To show" || got[0].Color != "#f59e0b" {
		t.Errorf("not tidied: %+v", got[0])
	}
	if got[1].ID == "" {
		t.Errorf("a new step got no id")
	}

	many := make([]models.Stage, MaxStages+1)
	for i := range many {
		many[i] = st(strings.Repeat("x", i+1), "s")
	}
	for name, bad := range map[string][]models.Stage{
		"empty name":   {st("a", "  ")},
		"long name":    {st("a", strings.Repeat("n", 41))},
		"bad id":       {st("a b", "s")},
		"duplicate id": {st("a", "one"), st("a", "two")},
		"bad colour":   {{ID: "a", Name: "s", Color: "orange"}},
		"too many":     many,
	} {
		if _, err := Clean(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v, want ErrInvalid", name, err)
		}
	}
}

func TestRemovedMovesTasksBack(t *testing.T) {
	old := []models.Stage{st("a", "A"), st("b", "B"), st("c", "C"), st("d", "D")}
	next := []models.Stage{st("c", "C"), st("new", "New")}
	got := Removed(old, next)
	// a and b had nothing kept before them: back to to do. d goes to c.
	want := map[string]string{"a": "", "b": "", "d": "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if len(Removed(old, old)) != 0 {
		t.Errorf("nothing removed, yet moves")
	}
}
