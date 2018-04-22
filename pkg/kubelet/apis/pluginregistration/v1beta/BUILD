load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

filegroup(
    name = "go_default_library_protos",
    srcs = ["api.proto"],
    visibility = ["//visibility:public"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "api.pb.go",
        "plugin_watcher.go",
        "validation.go",
    ],
    importpath = "k8s.io/kubernetes/pkg/kubelet/apis/pluginregistration/v1beta",
    visibility = ["//visibility:public"],
    deps = [
        "//pkg/apis/core/v1/helper:go_default_library",
        "//pkg/util/filesystem:go_default_library",
        "//vendor/github.com/fsnotify/fsnotify:go_default_library",
        "//vendor/github.com/gogo/protobuf/gogoproto:go_default_library",
        "//vendor/github.com/gogo/protobuf/proto:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/google.golang.org/grpc:go_default_library",
        "//vendor/k8s.io/api/core/v1:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["plugin_watcher_test.go"],
    importpath = "k8s.io/kubernetes/pkg/kubelet/apis/pluginregistration/v1beta",
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/fsnotify/fsnotify:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/stretchr/testify/require:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
