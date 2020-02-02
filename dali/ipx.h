
struct ipx_address {
	unsigned char network[4];
	unsigned char node[6];
	unsigned short socket;
};

struct ipx_header {
	unsigned short checksum;
	unsigned short length;
	unsigned char transport_control;
	unsigned char type;

	struct ipx_address dest, src;
};

struct ipx_ecb_fragment {
	unsigned short off, seg;
	unsigned short size;
};

struct ipx_ecb {
	unsigned short link[2];
	unsigned short esr_address[2];
	unsigned char in_use;
	unsigned char completion_code;
	unsigned short socket;
	unsigned char ipx_workspace[4];
	unsigned char driver_workspace[12];
	unsigned char immediate_address[6];
	unsigned short fragment_count;
	
	struct ipx_ecb_fragment fragments[1]; // [fragment_count]
};

