FROM nginx:alpine

# Routing rules: the same nginx.conf used by nginx.dockerfile and documented in
# docs/DEPLOYMENT-SERVERS.md. Unlike Apache (whose .htaccess is rsynced with the build),
# nginx routing lives on the server, so it is baked in here rather than deployed.
COPY nginx.conf /etc/nginx/conf.d/default.conf

# Start from an empty document root — rsync fills it. The stock nginx image ships an
# index.html here, which would mask a deploy that failed to transfer the real one.
RUN rm -f /usr/share/nginx/html/index.html /usr/share/nginx/html/50x.html

# Install OpenSSH and rsync (rsync is required on both sender and receiver)
# Generate host keys and create sshd privilege-separation dir
RUN apk add --no-cache openssh rsync \
    && ssh-keygen -A \
    && mkdir -p /run/sshd

# Allow root login with key auth; disable password auth.
# Alpine ships root locked ("!" in /etc/shadow), which sshd rejects even for pubkey auth
# (platform_locked_account). "*" is not treated as locked, so key auth is allowed.
RUN printf '\nPermitRootLogin prohibit-password\nPasswordAuthentication no\n' >> /etc/ssh/sshd_config \
    && sed -i 's/^root:!/root:*/' /etc/shadow

# Bake in the test public key (private key lives in web/testdata/rsync-test-key)
RUN mkdir -p /root/.ssh && chmod 700 /root/.ssh
COPY testdata/rsync-test-key.pub /root/.ssh/authorized_keys
RUN chmod 600 /root/.ssh/authorized_keys

COPY nginx-ssh-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
CMD ["/entrypoint.sh"]

# Runtime ports: 80 (nginx), 22 (SSH for rsync)
# Test key: web/testdata/rsync-test-key — local Docker testing only, not a production credential
