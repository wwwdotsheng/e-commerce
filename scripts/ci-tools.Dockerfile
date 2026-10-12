ARG GO_TEST_IMAGE=golang:1.26
FROM ${GO_TEST_IMAGE}
COPY --chmod=0755 kubectl /usr/local/bin/kubectl
RUN go version && kubectl version --client
