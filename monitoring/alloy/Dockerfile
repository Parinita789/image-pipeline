# Custom Alloy image with config
FROM grafana/alloy:latest
COPY config.river /etc/alloy/config.river
ENTRYPOINT ["alloy", "run", "--server.http.listen-addr=0.0.0.0:12345", "/etc/alloy/config.river"]