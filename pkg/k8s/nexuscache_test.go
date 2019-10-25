// +build unit

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNexusCache(t *testing.T) {
	assertForwardCache := func(t *testing.T, expected, actual map[string]Nexus) {
		// if used for maps pretty print
		if len(expected) != len(actual) {
			assert.Equal(t, expected, actual)
			return
		}
		for expKey, expVal := range expected {
			ok := assert.Contains(t, actual, expKey)
			if !ok {
				continue
			}
			actVal := actual[expKey]
			assert.Equal(t, expVal.AppName, actVal.AppName)
			assert.ElementsMatch(t, expVal.Services, actVal.Services)
		}
	}

	assertReverseCache := func(t *testing.T, expected, actual map[QualifiedName][]string) {
		// if used for maps pretty print
		if len(expected) != len(actual) {
			assert.Equal(t, expected, actual)
			return
		}
		for expKey, expVal := range expected {
			ok := assert.Contains(t, actual, expKey)
			if !ok {
				continue
			}
			actVal := actual[expKey]
			assert.ElementsMatch(t, expVal, actVal)
		}
	}

	type args struct {
		appName  string
		partID   string
		services []QualifiedName
	}
	type snapshot struct {
		forward map[string]Nexus
		reverse map[QualifiedName][]string
	}
	type operation string
	const (
		update operation = "update"
		delete operation = "delete"
	)
	cache := NewNexusCache()

	app1Name := "test-service"
	app2Name := "service"

	app1Service := NewQualifiedName(app1Name, "")
	app2Service := NewQualifiedName(app2Name, "")

	testDep1 := NewQualifiedName("test-dep-1", "")
	testDep21 := NewQualifiedName("test-dep-2", "ooh")
	testDep22 := NewQualifiedName("test-dep-2", "oooh")
	testDep23 := NewQualifiedName("test-dep-2", "oooooh")

	dep1 := NewQualifiedName("dep-1", "oooooh")
	dep2 := NewQualifiedName("dep-2", "eeh")

	tests := []struct {
		name         string
		op           operation
		cache        NexusCache
		args         args
		want         []string
		wantSnapshot snapshot
	}{
		{
			name:  "update empty cache",
			op:    update,
			cache: cache,
			args: args{
				appName: app1Name,
				partID:  app1Name + "-v1",
				services: []QualifiedName{
					app1Service,
					testDep1,
					testDep21,
					testDep22,
				},
			},
			want: []string{app1Name},
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
							testDep21,
							testDep22,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name},
					testDep21:   []string{app1Name},
					testDep22:   []string{app1Name},
				},
			},
		},
		{
			name:  "update cache with new part",
			op:    update,
			cache: cache,
			args: args{
				appName: app1Name,
				partID:  app1Name + "-v2",
				services: []QualifiedName{
					app1Service,
					testDep1,
					testDep23,
				},
			},
			want: []string{"test-service"},
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
							testDep21,
							testDep22,
							testDep23,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name},
					testDep21:   []string{app1Name},
					testDep22:   []string{app1Name},
					testDep23:   []string{app1Name},
				},
			},
		},
		{
			name:  "update part",
			op:    update,
			cache: cache,
			args: args{
				appName: app1Name,
				partID:  app1Name + "-v1",
				services: []QualifiedName{
					app1Service,
					testDep1,
				},
			},
			want: []string{"test-service"},
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
							testDep23,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name},
					testDep23:   []string{app1Name},
				},
			},
		},
		{
			name:  "update cache with same data",
			op:    update,
			cache: cache,
			args: args{
				appName: app1Name,
				partID:  app1Name + "-v2",
				services: []QualifiedName{
					app1Service,
					testDep1,
					testDep23,
				},
			},
			want: nil,
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
							testDep23,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name},
					testDep23:   []string{app1Name},
				},
			},
		},
		{
			name:  "update cache with new appID",
			op:    update,
			cache: cache,
			args: args{
				appName: app2Name,
				partID:  app2Name + "-v1",
				services: []QualifiedName{
					app2Service,
					dep1,
					dep2,
				},
			},
			want: []string{app2Name},
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
							testDep23,
						},
					},
					app2Name: Nexus{
						AppName: app2Name,
						Services: []QualifiedName{
							app2Service,
							dep1,
							dep2,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name},
					testDep23:   []string{app1Name},
					app2Service: []string{app2Name},
					dep1:        []string{app2Name},
					dep2:        []string{app2Name},
				},
			},
		},
		{
			name:  "update cache with used nexuses",
			op:    update,
			cache: cache,
			args: args{
				appName: app2Name,
				partID:  app2Name + "-v2",
				services: []QualifiedName{
					app2Service,
					dep1,
					testDep1,
					testDep23,
				},
			},
			want: []string{app2Name},
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
							testDep23,
						},
					},
					app2Name: Nexus{
						AppName: app2Name,
						Services: []QualifiedName{
							app2Service,
							dep1,
							dep2,
							testDep1,
							testDep23,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name, app2Name},
					testDep23:   []string{app1Name, app2Name},
					app2Service: []string{app2Name},
					dep1:        []string{app2Name},
					dep2:        []string{app2Name},
				},
			},
		},
		{
			name:  "delete part",
			op:    delete,
			cache: cache,
			args: args{
				appName: app2Name,
				partID:  app2Name + "-v2",
			},
			want: []string{app2Name},
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
							testDep23,
						},
					},
					app2Name: Nexus{
						AppName: app2Name,
						Services: []QualifiedName{
							app2Service,
							dep1,
							dep2,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name},
					testDep23:   []string{app1Name},
					app2Service: []string{app2Name},
					dep1:        []string{app2Name},
					dep2:        []string{app2Name},
				},
			},
		},
		{
			name:  "delete deleted part",
			op:    delete,
			cache: cache,
			args: args{
				appName: app2Name,
				partID:  app2Name + "-v2",
			},
			want: nil,
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
							testDep23,
						},
					},
					app2Name: Nexus{
						AppName: app2Name,
						Services: []QualifiedName{
							app2Service,
							dep1,
							dep2,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name},
					testDep23:   []string{app1Name},
					app2Service: []string{app2Name},
					dep1:        []string{app2Name},
					dep2:        []string{app2Name},
				},
			},
		},
		{
			name:  "delete last part in appID",
			op:    delete,
			cache: cache,
			args: args{
				appName: app2Name,
				partID:  app2Name + "-v1",
			},
			want: []string{app2Name},
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
							testDep23,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name},
					testDep23:   []string{app1Name},
				},
			},
		},
		{
			name:  "delete part 2",
			op:    delete,
			cache: cache,
			args: args{
				appName: app1Name,
				partID:  app1Name + "-v2",
			},
			want: []string{app1Name},
			wantSnapshot: snapshot{
				forward: map[string]Nexus{
					app1Name: Nexus{
						AppName: app1Name,
						Services: []QualifiedName{
							app1Service,
							testDep1,
						},
					},
				},
				reverse: map[QualifiedName][]string{
					app1Service: []string{app1Name},
					testDep1:    []string{app1Name},
				},
			},
		},
		{
			name:  "delete all",
			op:    delete,
			cache: cache,
			args: args{
				appName: app1Name,
				partID:  app1Name + "-v1",
			},
			want: []string{app1Name},
			wantSnapshot: snapshot{
				forward: map[string]Nexus{},
				reverse: map[QualifiedName][]string{},
			},
		},
		{
			name:  "delete part in empty cashe",
			op:    delete,
			cache: cache,
			args: args{
				appName: app1Name,
				partID:  app1Name + "-v1",
			},
			want: nil,
			wantSnapshot: snapshot{
				forward: map[string]Nexus{},
				reverse: map[QualifiedName][]string{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			if tt.op == update {
				got = tt.cache.Update(tt.args.appName, tt.args.partID, tt.args.services)
			} else {
				got = tt.cache.Delete(tt.args.appName, tt.args.partID)
			}
			assert.Equal(t, tt.want, got)
			forwardSnap, reverseSnap := tt.cache.GetSnapshot()
			assertForwardCache(t, tt.wantSnapshot.forward, forwardSnap)
			assertReverseCache(t, tt.wantSnapshot.reverse, reverseSnap)
		})
	}
}
