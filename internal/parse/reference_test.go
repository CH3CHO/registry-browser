package parse

import "testing"

// Q18 决策：解析器必须覆盖下列全部边界，且必须是纯函数、可单测。
// 参考用例来源：higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/all-in-one:2.2.6

func TestParse(t *testing.T) {
	known := Options{
		KnownRegistries: map[string]struct{}{
			"harbor.internal.example": {},
			"harbor":                  {},
			"localhost:5000":          {},
		},
	}

	cases := []struct {
		name    string
		input   string
		opts    Options
		want    Reference
		wantErr bool
	}{
		// ---------- Docker Hub 官方库补全 ----------
		{
			name:  "bare official image gets docker.io/library prefix",
			input: "nginx",
			want: Reference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Namespace:  "library",
				Image:      "nginx",
			},
		},
		{
			name:  "bare official image with tag",
			input: "nginx:1.25.3",
			want: Reference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Namespace:  "library",
				Image:      "nginx",
				Tag:        "1.25.3",
			},
		},
		{
			name:  "explicit library namespace",
			input: "library/nginx",
			want: Reference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Namespace:  "library",
				Image:      "nginx",
			},
		},
		{
			name:  "docker.io prefix is explicit and preserved",
			input: "docker.io/library/nginx:1.27",
			want: Reference{
				Registry:   "docker.io",
				Repository: "library/nginx",
				Namespace:  "library",
				Image:      "nginx",
				Tag:        "1.27",
			},
		},

		// ---------- 用户真实场景：ACR ----------
		{
			name:  "acr host with two level path and tag",
			input: "higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/all-in-one:2.2.6",
			want: Reference{
				Registry:   "higress-registry.cn-hangzhou.cr.aliyuncs.com",
				Repository: "higress/all-in-one",
				Namespace:  "higress",
				Image:      "all-in-one",
				Tag:        "2.2.6",
			},
		},
		{
			name:  "acr host without tag",
			input: "higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/all-in-one",
			want: Reference{
				Registry:   "higress-registry.cn-hangzhou.cr.aliyuncs.com",
				Repository: "higress/all-in-one",
				Namespace:  "higress",
				Image:      "all-in-one",
			},
		},
		{
			name:  "three level path collapses namespace to all but last segment",
			input: "registry.example.com/team/sub/app:v2",
			want: Reference{
				Registry:   "registry.example.com",
				Repository: "team/sub/app",
				Namespace:  "team/sub",
				Image:      "app",
				Tag:        "v2",
			},
		},

		// ---------- 端口 vs tag 的歧义 ----------
		{
			name:  "registry with port is not a tag",
			input: "localhost:5000/repo/img:1.0",
			want: Reference{
				Registry:   "localhost:5000",
				Repository: "repo/img",
				Namespace:  "repo",
				Image:      "img",
				Tag:        "1.0",
			},
		},
		{
			name:  "registry with port and no tag",
			input: "localhost:5000/repo/img",
			want: Reference{
				Registry:   "localhost:5000",
				Repository: "repo/img",
				Namespace:  "repo",
				Image:      "img",
			},
		},

		// ---------- digest ----------
		{
			name:  "digest reference has no tag",
			input: "ubuntu@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			want: Reference{
				Registry:   "docker.io",
				Repository: "library/ubuntu",
				Namespace:  "library",
				Image:      "ubuntu",
				Digest:     "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		},
		{
			name:  "tag and digest together",
			input: "ubuntu:22.04@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			want: Reference{
				Registry:   "docker.io",
				Repository: "library/ubuntu",
				Namespace:  "library",
				Image:      "ubuntu",
				Tag:        "22.04",
				Digest:     "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
			},
		},

		// ---------- 已知 registry 覆盖：无点无冒号的首段也可能是 registry ----------
		{
			name:  "known registry without dot is treated as host",
			input: "harbor.internal.example/team/app:1",
			opts:  known,
			want: Reference{
				Registry:   "harbor.internal.example",
				Repository: "team/app",
				Namespace:  "team",
				Image:      "app",
				Tag:        "1",
			},
		},
		{
			name:  "known registry with port is treated as host",
			input: "localhost:5000/team/app",
			opts:  known,
			want: Reference{
				Registry:   "localhost:5000",
				Repository: "team/app",
				Namespace:  "team",
				Image:      "app",
			},
		},
		{
			name:  "unknown dotless first segment stays a namespace on docker.io",
			input: "higress/all-in-one:2.2.6",
			want: Reference{
				Registry:   "docker.io",
				Repository: "higress/all-in-one",
				Namespace:  "higress",
				Image:      "all-in-one",
				Tag:        "2.2.6",
			},
		},
		{
			name:  "dotless host registered as known registry",
			input: "harbor/team/app:1",
			opts:  known,
			want: Reference{
				Registry:   "harbor",
				Repository: "team/app",
				Namespace:  "team",
				Image:      "app",
				Tag:        "1",
			},
		},

		// ---------- 错误输入 ----------
		{name: "empty input", input: "", wantErr: true},
		{name: "blank input", input: "   ", wantErr: true},
		{name: "uppercase repository", input: "Library/Nginx", wantErr: true},
		{name: "empty tag after colon", input: "nginx:", wantErr: true},
		{name: "empty image after slash", input: "repo/", wantErr: true},
		{name: "leading slash", input: "/repo/img", wantErr: true},
		{name: "invalid digest algorithm", input: "ubuntu@md5:abc", wantErr: true},
		{name: "invalid digest hex length", input: "ubuntu@sha256:abc", wantErr: true},
		{name: "invalid tag characters", input: "nginx:in valid", wantErr: true},
		{name: "tag too long", input: "nginx:" + longString(129), wantErr: true},
		{name: "invalid repository characters", input: "repo/img!name", wantErr: true},
		{name: "digest without algorithm separator", input: "ubuntu@0123456789abcdef", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.input, tc.opts)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) expected error, got %+v", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("Parse(%q)\n got: %+v\nwant: %+v", tc.input, got, tc.want)
			}
		})
	}
}

func TestParseRegistryHost(t *testing.T) {
	// registry 主机名识别规则：含 '.' 或 ':' 即视为 host；否则是命名空间。
	cases := []struct {
		input string
		want  string
	}{
		{"registry.example.com/a/b", "registry.example.com"},
		{"registry.example.com:5000/a/b", "registry.example.com:5000"},
		{"higress-registry.cn-hangzhou.cr.aliyuncs.com/higress/all-in-one", "higress-registry.cn-hangzhou.cr.aliyuncs.com"},
		{"a/b/c", ""},
		{"a", ""},
	}

	for _, tc := range cases {
		got := registryHostOf(tc.input)
		if got != tc.want {
			t.Errorf("registryHostOf(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseDoesNotMutateInput(t *testing.T) {
	input := "registry.example.com/team/app:v1"
	original := input
	if _, err := Parse(input, Options{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if input != original {
		t.Errorf("Parse mutated its input: %q -> %q", original, input)
	}
}

func longString(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
