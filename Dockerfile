FROM alpine
WORKDIR /
COPY gpu-admission /gpu-admission
CMD ["./gpu-admission"]