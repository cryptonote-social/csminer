#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include "niceapi.h"

int main(int argc, char* argv[]) {
  pool_login_args args;
  args.username = "cryptonote-social";
  if (argc > 1) {
	printf("using arg for username: %s\n", argv[1]);
	args.username = argv[1];
  }
  args.rigid = NULL;

  args.wallet = NULL;
  if (argc > 2) {
	printf("using arg for wallet: %s\n", argv[2]);
	args.wallet = argv[2];
  }

  args.agent = "Super Power Ultimate Miner (S.P.U.M.) v0.6.9";

  args.config = NULL;
  
  pool_login_response r;
  r = pool_login(&args);
  if (r.code < 0) {
	printf("Oh no, login failed: %s\n", r.message);
	free((void*)r.message);
	return 1;
  }
  if (r.code > 1) {
	printf("Pool server didn't like login info: %s\n", r.message);
	free((void*)r.message);
	return 2;
  }
  printf("Successful login.\n");
  if (r.message) {
	printf("   Pool returned warning: %s\n", r.message);
	free((void*)r.message);
  }
  return 0;
}

