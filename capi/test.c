#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include "niceapi.h"

int main(int argc, char* argv[]) {
  // Pool login...
  pool_login_args pl_args;
  pl_args.username = "cryptonote-social";
  if (argc > 1) {
	printf("using arg for username: %s\n", argv[1]);
	pl_args.username = argv[1];
  }
  pl_args.rigid = NULL;

  pl_args.wallet = NULL;
  if (argc > 2) {
	printf("using arg for wallet: %s\n", argv[2]);
	pl_args.wallet = argv[2];
  }

  pl_args.agent = "Super Power Ultimate Miner (S.P.U.M.) v0.6.9";

  pl_args.config = NULL;
  
  pool_login_response pl_resp = pool_login(&pl_args);
  if (pl_resp.code < 0) {
	printf("Oh no, login failed: %s\n", pl_resp.message);
	free((void*)pl_resp.message);
	return 1;
  }
  if (pl_resp.code > 1) {
	printf("Pool server didn't like login info: %s\n", pl_resp.message);
	free((void*)pl_resp.message);
	return 2;
  }
  printf("Successful login.\n");
  if (pl_resp.message) {
	printf("   Pool returned warning: %s\n", pl_resp.message);
	free((void*)pl_resp.message);
  }

  // Starting the miner....
  start_miner_args sm_args;
  sm_args.threads = 1;
  sm_args.exclude_hour_start = 27;
  sm_args.exclude_hour_end = 0;

  start_miner_response sm_resp = start_miner(&sm_args);
  if (sm_resp.code > 2) {
    printf("Bad config options specified: %s\n", sm_resp.message);
    free((void*)sm_resp.message);
    return 3;
  }
  if (sm_resp.code < 0) {
    printf("Unrecoverable error: %s\n", sm_resp.message);
    free((void*)sm_resp.message);
    return 4;
  }
  if (sm_resp.code == 2) {
    printf("Huge Pages could not be enabled -- mining may be slow. Consider restarting your machine and trying again.\n");
  } 
  printf("Miner started.\n");
  return 0;
}
