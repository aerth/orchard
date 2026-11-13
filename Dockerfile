FROM debian:trixie-slim
RUN apt update && apt upgrade -y && apt install -y bash bat gum
RUN useradd username
USER 1000:1000
COPY ./tmp/otools /home/username/src/otools
COPY ./bin/orchard-io /bin/
ENTRYPOINT ["/bin/orchard-io"]
