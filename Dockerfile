FROM golang

# Install koyeb-cli
RUN curl -fsSL https://raw.githubusercontent.com/koyeb/koyeb-cli/master/install.sh | sh
ENV PATH="$PATH:/root/.koyeb/bin"

WORKDIR /app

COPY . .
RUN go install ./cmd/executor

CMD ["/go/bin/executor"]