package procenv

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestWrite(t *testing.T) {
	cases := []struct {
		name    string
		debug   string
		environ []string
		want    string
		wantErr bool
	}{
		{name: "unset", debug: "", environ: []string{"A=1"}, wantErr: true},
		{name: "zero", debug: "0", environ: []string{"A=1"}, wantErr: true},
		{name: "true", debug: "true", environ: []string{"A=1"}, wantErr: true},
		{name: "yes", debug: "yes", environ: []string{"A=1"}, wantErr: true},
		{name: "two", debug: "2", environ: []string{"A=1"}, wantErr: true},
		{name: "padded", debug: " 1", environ: []string{"A=1"}, wantErr: true},
		{name: "empty environ", debug: "1", environ: nil, want: ""},
		{name: "single", debug: "1", environ: []string{"PATH=/bin"}, want: "PATH=/bin\n"},
		{name: "sorted", debug: "1", environ: []string{"ZED=9", "A=1", "M=5"}, want: "A=1\nM=5\nZED=9\n"},
		{name: "keeps equals in value", debug: "1", environ: []string{"B=x=y", "A=1"}, want: "A=1\nB=x=y\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := Write(&buf, tc.debug, tc.environ)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), DebugVar) {
					t.Errorf("error %q does not mention %s", err, DebugVar)
				}
				if buf.Len() != 0 {
					t.Errorf("wrote %q while locked", buf.String())
				}
				return
			}
			if err != nil {
				t.Fatalf("Write: %v", err)
			}
			if got := buf.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

type errWriter struct{}

func (errWriter) Write([]byte) (int, error) {
	return 0, errors.New("sink is full")
}

func TestWrite_WriterError(t *testing.T) {
	err := Write(errWriter{}, "1", []string{"A=1"})
	if err == nil {
		t.Fatal("expected write error")
	}
	if !strings.Contains(err.Error(), "sink is full") {
		t.Errorf("got %v", err)
	}
}
