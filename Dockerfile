FROM registry.access.redhat.com/ubi9/ubi-minimal:latest

RUN microdnf install -y ca-certificates && microdnf clean all

ARG TARGETARCH=amd64
COPY dist/linux_${TARGETARCH}/mssql-mcp-go /usr/local/bin/mssql-mcp-go

EXPOSE 8080 8081

USER 1000:1000

ENTRYPOINT ["/usr/local/bin/mssql-mcp-go"]