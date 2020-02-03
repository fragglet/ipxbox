
#include <stdio.h>
#include <stdlib.h>

#include "dbipx.h"

int main(int argc, char *argv[])
{
	char addr[6];

	if (argc < 3) {
		fprintf(stderr, "Usage: %s <addr> <port>\n", argv[0]);
		exit(1);
	}

	DBIPX_Connect(argv[1], atoi(argv[2]));
	printf("Connected successfully!\n");
	DBIPX_GetAddress(addr);
	printf("Assigned address is %02x:%02x:%02x:%02x:%02x:%02x.\n",
	       addr[0], addr[1], addr[2], addr[3], addr[4], addr[5]);
	return 0;
}

