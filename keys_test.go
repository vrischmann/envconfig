package envconfig

import "testing"

func TestMakeAllPossibleKeys(t *testing.T) {
	fieldName := "CassandraSslCert"
	keys := makeAllPossibleKeys(&context{
		name: fieldName,
	})

	equals(t, 4, len(keys))
	equals(t, "CASSANDRASSLCERT", keys[0])
	equals(t, "CASSANDRA_SSL_CERT", keys[1])
	equals(t, "cassandra_ssl_cert", keys[2])
	equals(t, "cassandrasslcert", keys[3])

	fieldName = "CassandraSSLCert"
	keys = makeAllPossibleKeys(&context{
		name: fieldName,
	})

	equals(t, 4, len(keys))
	equals(t, "CASSANDRASSLCERT", keys[0])
	equals(t, "CASSANDRA_SSL_CERT", keys[1])
	equals(t, "cassandra_ssl_cert", keys[2])
	equals(t, "cassandrasslcert", keys[3])

	fieldName = "Cassandra.SslCert"
	keys = makeAllPossibleKeys(&context{
		name: fieldName,
	})

	equals(t, 4, len(keys))
	equals(t, "CASSANDRA_SSLCERT", keys[0])
	equals(t, "CASSANDRA_SSL_CERT", keys[1])
	equals(t, "cassandra_ssl_cert", keys[2])
	equals(t, "cassandra_sslcert", keys[3])

	fieldName = "Cassandra.SSLCert"
	keys = makeAllPossibleKeys(&context{
		name: fieldName,
	})

	equals(t, 4, len(keys))
	equals(t, "CASSANDRA_SSLCERT", keys[0])
	equals(t, "CASSANDRA_SSL_CERT", keys[1])
	equals(t, "cassandra_ssl_cert", keys[2])
	equals(t, "cassandra_sslcert", keys[3])

	fieldName = "Name"
	keys = makeAllPossibleKeys(&context{
		name: fieldName,
	})

	equals(t, 2, len(keys))
	equals(t, "NAME", keys[0])
	equals(t, "name", keys[1])
}
