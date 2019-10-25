// +build unit

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNexusIsEqual(t *testing.T) {

	app1 := "test-service"
	app2 := "service"

	dep11 := NewQualifiedName("dep1", "")

	dep21 := NewQualifiedName("dep2", "eeh")
	dep22 := NewQualifiedName("dep2", "ooh")
	dep23 := NewQualifiedName("dep2", "oooh")

	dep31 := NewQualifiedName("dep3", "")

	tests := []struct {
		name  string
		nexus Nexus
		arg   Nexus
		want  bool
	}{
		{
			name:  "equal simple",
			nexus: NewNexus(app1, []QualifiedName{dep11}),
			arg:   NewNexus(app1, []QualifiedName{dep11}),
			want:  true,
		},
		{
			name:  "not equal simple",
			nexus: NewNexus(app1, []QualifiedName{dep11}),
			arg:   NewNexus(app2, []QualifiedName{dep21}),
			want:  false,
		},
		{
			name:  "equal with nil",
			nexus: NewNexus(app1, nil),
			arg:   NewNexus(app1, nil),
			want:  true,
		},
		{
			name:  "not equal with nil",
			nexus: NewNexus(app1, nil),
			arg:   NewNexus(app1, []QualifiedName{dep11}),
			want:  false,
		},
		{
			name: "equal several services",
			nexus: NewNexus(app1, []QualifiedName{
				dep11,
				dep21,
				dep22,
				dep23,
			}),
			arg: NewNexus(app1, []QualifiedName{
				dep11,
				dep21,
				dep22,
				dep23,
			}),
			want: true,
		},
		{
			name: "not equal several services",
			nexus: NewNexus(app1, []QualifiedName{
				dep11,
				dep21,
				dep22,
				dep23,
			}),
			arg: NewNexus(app1, []QualifiedName{
				dep21,
				dep22,
				dep23,
				dep31,
			}),
			want: false,
		},
		{
			name: "not equal disjoint services",
			nexus: NewNexus(app1, []QualifiedName{
				dep11,
				dep21,
			}),
			arg: NewNexus(app1, []QualifiedName{
				dep22,
				dep23,
				dep31,
			}),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.nexus.isEqual(tt.arg); got != tt.want {
				t.Errorf("Nexus.isEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPartitionedNexus(t *testing.T) {
	type args struct {
		partID   string
		services []QualifiedName
	}
	type operation string
	const (
		update operation = "update"
		remove operation = "remove"
	)

	app := "test-service"

	part1 := app + "-v1"
	part2 := app + "-v2"

	dep1 := NewQualifiedName("dep1", "")
	dep2 := NewQualifiedName("dep2", "ooh")
	dep3 := NewQualifiedName("dep3", "")

	appNexus := newPartitionNexus(app)

	tests := []struct {
		name      string
		op        operation
		partNexus *partitionedNexus
		args      args
		want      bool
		wantNexus Nexus
	}{
		{
			name:      "update empty nexus with empty services list",
			op:        update,
			partNexus: appNexus,
			args: args{
				partID:   part1,
				services: []QualifiedName{},
			},
			want: false,
			wantNexus: Nexus{
				AppName:  app,
				Services: nil,
			},
		},
		{
			name:      "update empty nexus with nil services list",
			op:        update,
			partNexus: appNexus,
			args: args{
				partID:   part1,
				services: nil,
			},
			want: false,
			wantNexus: Nexus{
				AppName:  app,
				Services: nil,
			},
		},
		{
			name:      "update empty nexus",
			op:        update,
			partNexus: appNexus,
			args: args{
				partID: part1,
				services: []QualifiedName{
					dep1,
				},
			},
			want: true,
			wantNexus: Nexus{
				AppName: app,
				Services: []QualifiedName{
					dep1,
				},
			},
		},
		{
			name:      "update nexus part",
			op:        update,
			partNexus: appNexus,
			args: args{
				partID: part1,
				services: []QualifiedName{
					dep1,
					dep2,
				},
			},
			want: true,
			wantNexus: Nexus{
				AppName: app,
				Services: []QualifiedName{
					dep1,
					dep2,
				},
			},
		},
		{
			name:      "update nexus with new part",
			op:        update,
			partNexus: appNexus,
			args: args{
				partID: part2,
				services: []QualifiedName{
					dep3,
				},
			},
			want: true,
			wantNexus: Nexus{
				AppName: app,
				Services: []QualifiedName{
					dep1,
					dep2,
					dep3,
				},
			},
		},
		{
			name:      "update nexus part with same data",
			op:        update,
			partNexus: appNexus,
			args: args{
				partID: part2,
				services: []QualifiedName{
					dep2,
				},
			},
			want: true,
			wantNexus: Nexus{
				AppName: app,
				Services: []QualifiedName{
					dep1,
					dep2,
				},
			},
		},
		{
			name:      "remove nexus part",
			op:        update,
			partNexus: appNexus,
			args: args{
				partID: part1,
			},
			want: true,
			wantNexus: Nexus{
				AppName: app,
				Services: []QualifiedName{
					dep2,
				},
			},
		},
		{
			name:      "remove nexus all parts",
			op:        update,
			partNexus: appNexus,
			args: args{
				partID: part2,
			},
			want: true,
			wantNexus: Nexus{
				AppName:  app,
				Services: nil,
			},
		},
		{
			name:      "remove removed nexus part",
			op:        update,
			partNexus: appNexus,
			args: args{
				partID: part2,
			},
			want: false,
			wantNexus: Nexus{
				AppName:  app,
				Services: nil,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got bool
			if tt.op == update {
				got = tt.partNexus.UpdateServices(tt.args.partID, tt.args.services)
			} else {
				got = tt.partNexus.RemoveServices(tt.args.partID)
			}
			assert.Equal(t, tt.want, got)
			n := tt.partNexus.getNexus()
			assert.Equal(t, tt.wantNexus.AppName, n.AppName)
			assert.ElementsMatch(t, tt.wantNexus.Services, n.Services)
		})
	}
}
