package app

import (
	"testing"
)

func TestGroupRepositories(t *testing.T) {
	cases := []struct {
		name         string
		repositories []string
		namespace    string
		want         []NamespaceEntry
	}{
		{
			name:         "two level paths group by first segment",
			repositories: []string{"higress/all-in-one", "higress/gateway", "team/app"},
			want: []NamespaceEntry{
				{
					Name: "higress",
					Images: []ImageEntry{
						{Name: "all-in-one", Repository: "higress/all-in-one"},
						{Name: "gateway", Repository: "higress/gateway"},
					},
				},
				{
					Name:   "team",
					Images: []ImageEntry{{Name: "app", Repository: "team/app"}},
				},
			},
		},
		{
			name:         "three level paths keep everything but the last segment",
			repositories: []string{"team/sub/app"},
			want: []NamespaceEntry{
				{
					Name:   "team/sub",
					Images: []ImageEntry{{Name: "app", Repository: "team/sub/app"}},
				},
			},
		},
		{
			name:         "single segment repository has no namespace",
			repositories: []string{"app"},
			want: []NamespaceEntry{
				{
					Name:   "",
					Images: []ImageEntry{{Name: "app", Repository: "app"}},
				},
			},
		},
		{
			name:         "configured namespace wins over path shape",
			repositories: []string{"higress/all-in-one", "other/thing"},
			namespace:    "higress",
			want: []NamespaceEntry{
				{
					Name: "higress",
					Images: []ImageEntry{
						{Name: "all-in-one", Repository: "higress/all-in-one"},
					},
				},
				{
					Name:   "other",
					Images: []ImageEntry{{Name: "thing", Repository: "other/thing"}},
				},
			},
		},
		{
			name:         "configured namespace matches repository exactly",
			repositories: []string{"higress"},
			namespace:    "higress",
			want: []NamespaceEntry{
				{
					Name:   "higress",
					Images: []ImageEntry{{Name: "higress", Repository: "higress"}},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := groupRepositories(tc.repositories, tc.namespace)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d namespaces %+v, want %d %+v", len(got), got, len(tc.want), tc.want)
			}
			for i := range tc.want {
				if got[i].Name != tc.want[i].Name {
					t.Fatalf("namespace[%d] = %q, want %q", i, got[i].Name, tc.want[i].Name)
				}
				if len(got[i].Images) != len(tc.want[i].Images) {
					t.Fatalf("namespace[%d] images = %+v, want %+v", i, got[i].Images, tc.want[i].Images)
				}
				for j := range tc.want[i].Images {
					if got[i].Images[j] != tc.want[i].Images[j] {
						t.Fatalf("image[%d][%d] = %+v, want %+v", i, j, got[i].Images[j], tc.want[i].Images[j])
					}
				}
			}
		})
	}
}

func TestGroupRepositoriesEmpty(t *testing.T) {
	got := groupRepositories(nil, "")
	if len(got) != 0 {
		t.Errorf("expected no namespaces, got %+v", got)
	}
}

func TestSplitGroup(t *testing.T) {
	cases := []struct {
		repository string
		namespace  string
		wantGroup  string
		wantImage  string
	}{
		{"team/app", "", "team", "app"},
		{"app", "", "", "app"},
		{"higress/all-in-one", "higress", "higress", "all-in-one"},
		{"other/thing", "higress", "other", "thing"},
	}

	for _, tc := range cases {
		group, image := splitGroup(tc.repository, tc.namespace)
		if group != tc.wantGroup || image != tc.wantImage {
			t.Errorf("splitGroup(%q, %q) = %q/%q, want %q/%q", tc.repository, tc.namespace, group, image, tc.wantGroup, tc.wantImage)
		}
	}
}
