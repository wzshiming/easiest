package easiest

import (
	"io"
	"reflect"
	"strings"
	"testing"
)

func Test_newReplaceReader(t *testing.T) {
	type args struct {
		r   io.Reader
		old []byte
		new []byte
	}
	tests := []struct {
		name string
		args args
		want []byte
	}{
		{
			name: "3 -> 3",
			args: args{
				r:   strings.NewReader("123456789"),
				old: []byte("456"),
				new: []byte("ABC"),
			},
			want: []byte("123ABC789"),
		},
		{
			name: "3 -> 2",
			args: args{
				r:   strings.NewReader("123456789"),
				old: []byte("456"),
				new: []byte("AB"),
			},
			want: []byte("123AB789"),
		},
		{
			name: "3 -> 4",
			args: args{
				r:   strings.NewReader("123456789"),
				old: []byte("456"),
				new: []byte("ABCD"),
			},
			want: []byte("123ABCD789"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newReplaceReader(tt.args.r, tt.args.old, tt.args.new, nil)
			gotData, _ := io.ReadAll(got)
			if !reflect.DeepEqual(gotData, tt.want) {
				t.Errorf("newReplaceReader() = %q, want %q", gotData, tt.want)
			}
		})
	}
}
