package(default_visibility = ["//kythe:default_visibility"])

java_library(
    name = "java",
    srcs = [
        "JavaCompilationDetails.java",
        "JavacAnalysisDriver.java",
        "JavacAnalyzer.java",
    ],
    deps = [
        ":options",
        "//kythe/java/com/google/devtools/kythe/common",
        "//kythe/java/com/google/devtools/kythe/platform/java/filemanager",
        "//kythe/java/com/google/devtools/kythe/platform/shared:analysis_exception",
        "//kythe/java/com/google/devtools/kythe/platform/shared:file_data",
        "//kythe/java/com/google/devtools/kythe/platform/shared:statistics",
        "//kythe/proto:analysis_proto_java",
        "//kythe/proto:storage_proto_java",
        "//third_party/guava",
        "//third_party/jsr305_annotations:jsr305",
        "//third_party/openjdk:tools",
        "//third_party/proto:protobuf_java",
    ],
)

java_library(
    name = "options",
    srcs = ["JavacOptionsUtils.java"],
    deps = [
        "//third_party/guava",
        "//third_party/jsr305_annotations:jsr305",
        "//third_party/openjdk:tools",
    ],
)
