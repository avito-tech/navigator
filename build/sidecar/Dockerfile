FROM envoyproxy/envoy:v1.12.2

COPY ./envoy /envoy
RUN chmod -R 777 /envoy

ENTRYPOINT ["/envoy/start.sh"]
