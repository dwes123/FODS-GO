# Run Stage
FROM alpine:latest
WORKDIR /root/
RUN apk --no-cache add ca-certificates
# We copy the 'server' file we just uploaded via SCP
COPY server .
COPY templates ./templates
# Ensure the file is executable
RUN chmod +x server
EXPOSE 8080
CMD ["./server"]