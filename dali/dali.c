
#include <stdio.h>
#include <stdlib.h>

#include <process.h>

#include "ipx.h"
#include "dbipx.h"

int main(int argc, char *argv[])
{
	char *addr;

	if (argc < 3) {
		fprintf(stderr, "Usage: %s <addr> <port>\n", argv[0]);
		exit(1);
	}

	DBIPX_Connect(argv[1], atoi(argv[2]));
	printf("Connected successfully!\n");
	addr = dbipx_local_addr.node;
	printf("Assigned address is %02x:%02x:%02x:%02x:%02x:%02x.\n",
	       addr[0], addr[1], addr[2], addr[3], addr[4], addr[5]);

	HookIPXVector();

	// "Poor man's TSR" where we run command.com
	// Temporary hack until we convert this to a real TSR
	{
		const char * const args[] = {"z:\\command.com", NULL};
		spawnv(P_WAIT, args[0], args);
	}
	printf("DALI exiting.\n");

	return 0;
}

