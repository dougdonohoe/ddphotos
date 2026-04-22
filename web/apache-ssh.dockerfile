FROM httpd:2.4

# Apache: enable mod_rewrite, AllowOverride for .htaccess (same config as apache.dockerfile)
RUN sed -i 's/#LoadModule rewrite_module/LoadModule rewrite_module/' /usr/local/apache2/conf/httpd.conf \
    && sed -i 's/AllowOverride None/AllowOverride All/g' /usr/local/apache2/conf/httpd.conf \
    && echo "ServerName localhost" >> /usr/local/apache2/conf/httpd.conf \
    && printf '\n<Directory "/usr/local/apache2/htdocs">\n    Options +FollowSymLinks\n</Directory>\n' >> /usr/local/apache2/conf/httpd.conf

# Install OpenSSH and rsync (rsync is required on both sender and receiver)
# Generate host keys and create sshd privilege-separation dir
RUN apt-get update && apt-get install -y --no-install-recommends openssh-server rsync \
    && rm -rf /var/lib/apt/lists/* \
    && ssh-keygen -A \
    && mkdir -p /run/sshd

# Allow root login with key auth; disable password auth
RUN printf '\nPermitRootLogin prohibit-password\nPasswordAuthentication no\n' >> /etc/ssh/sshd_config

# Bake in the test public key (private key lives in web/testdata/rsync-test-key)
RUN mkdir -p /root/.ssh && chmod 700 /root/.ssh
COPY testdata/rsync-test-key.pub /root/.ssh/authorized_keys
RUN chmod 600 /root/.ssh/authorized_keys

COPY apache-ssh-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
CMD ["/entrypoint.sh"]

# Runtime ports: 80 (Apache), 22 (SSH for rsync)
# Test key: web/testdata/rsync-test-key — local Docker testing only, not a production credential
