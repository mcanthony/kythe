load("/tools/build_rules/go", "go_binary")

package(default_visibility = ["//kythe:default_visibility"])

go_binary(
    name = "http_server",
    srcs = [
        "http_server/http_server.go",
    ],
    deps = [
        "//kythe/go/services/filetree",
        "//kythe/go/services/search",
        "//kythe/go/services/web",
        "//kythe/go/services/xrefs",
        "//kythe/go/serving/filetree",
        "//kythe/go/serving/search",
        "//kythe/go/serving/xrefs",
        "//kythe/go/storage/leveldb",
        "//kythe/go/storage/table",
        "//third_party/go:context",
    ],
)
