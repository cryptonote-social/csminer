#include <stddef.h>
#include <stdlib.h>
#include <stdio.h>
#include <unistd.h>
#include "niceapi.h"

int main(int argc, char* argv[]) {
  // Miner initialization
  init_miner_args sm_args;
  sm_args.threads = 1;
  sm_args.exclude_hour_start = 0;
  sm_args.exclude_hour_end = 0;

  init_miner_response sm_resp = init_miner(&sm_args);
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
  printf("Miner initialized.\n");

  report_lock_screen_state(true); // pretend screen is locked so we will mine

  pool_login_args pl_args;
  pl_args.agent = "csminer / minerlib test script";
  pl_args.rigid = NULL;
  pl_args.wallet = NULL;
  pl_args.config = NULL;

  get_miner_state_response ms_resp;

  // Login loop. Alternate between 2 accounts every minute to make sure account switching works.
  while (true) {
    printf("Entering get_miner_state polling loop, 30 polls with 1 second sleep inbetween\n");
    for (int i = 0; i < 30; ++i) {
        ms_resp = get_miner_state();
        printf("Hashrate was: %f\n", ms_resp.recent_hashrate);
        printf("Threads active: %d\n", ms_resp.threads);
        printf("Mining activity state: %d\n", ms_resp.mining_activity);
        free((void*)ms_resp.username);
        free((void*)ms_resp.time_to_reward);
		increase_threads();
        sleep(.5);
		decrease_threads();		
        sleep(.5);
    }


    pl_args.username = "cryptonote-social";
    if (argc > 1) {
      printf("using arg for username: %s\n", argv[1]);
      pl_args.username = argv[1];
    }
    if (argc > 2) {
      printf("using arg for wallet: %s\n", argv[2]);
      pl_args.wallet = argv[2];
    }
    printf("Logging in with user: %s\n", pl_args.username);
    pool_login_response pl_resp = pool_login(&pl_args);
    if (pl_resp.code < 0) {
      printf("Oh no, login failed: %s\n", pl_resp.message);
    }
    if (pl_resp.code > 1) {
      printf("Pool server didn't like login info: %s\n", pl_resp.message);
    }
    if (pl_resp.code == 1) {
	  printf("Successful login #1.\n");
	  if (strlen(pl_resp.message) > 0) {
		printf("   Pool returned warning: %s\n", pl_resp.message);
	  }
	}
	free((void*)pl_resp.message);
	
	send_chat("testing chat sending this is the chat message");

	printf("Entering get_miner_state polling loop, 60 polls with 1 second sleep inbetween\n");
    for (int i = 0; i < 60; ++i) {
        ms_resp = get_miner_state();
        printf("Hashrate was: %f\n", ms_resp.recent_hashrate);
        printf("Threads active: %d\n", ms_resp.threads);
        printf("Mining activity state: %d\n", ms_resp.mining_activity);
        printf("Chats available: %s\n", ms_resp.chats_available ? "yes" : "no");
		if (ms_resp.chats_available) {
		  next_chat_response nc_resp;
		  nc_resp = next_chat();
		  printf("Got chat message: [ %s ] %s  (%ld)\n", nc_resp.username, nc_resp.message, nc_resp.timestamp);
		  free((void*)nc_resp.username);
		  free((void*)nc_resp.message);
		}
        free((void*)ms_resp.username);
        free((void*)ms_resp.time_to_reward);
        sleep(1);
    }


    sleep(10);
    printf("Setting screen state to active\n");
    report_lock_screen_state(false/*is_locked*/);  // make sure miner pauses
    sleep(10);
    printf("Setting screen state to locked\n");
    report_lock_screen_state(true);
    sleep(10);
    printf("Setting power state to on-battery\n");
    report_power_state(true/*on_battery*/);  // make sure miner pauses
    sleep(10);
    printf("Setting power state to power adapter\n");
    report_power_state(false);

    printf("Sleeping for 2 minutes before trying another login.\n");
    sleep(30);
	ms_resp = get_miner_state();
	printf("Hashrate was: %f\n", ms_resp.recent_hashrate);
	printf("Threads active: %d\n", ms_resp.threads);
    printf("Mining activity state: %d\n", ms_resp.mining_activity);
	free((void*)ms_resp.username);
	free((void*)ms_resp.time_to_reward);

    printf("Increasing threads\n");
    increase_threads();

    printf("Entering get_miner_state polling loop, 60 polls with 1 second sleep inbetween\n");
    for (int i = 0; i < 60; ++i) {
        ms_resp = get_miner_state();
        printf("Hashrate was: %f\n", ms_resp.recent_hashrate);
        printf("Threads active: %d\n", ms_resp.threads);
        printf("Mining activity state: %d\n", ms_resp.mining_activity);
        printf("Chats available: %s\n", ms_resp.chats_available ? "yes" : "no");
		if (ms_resp.chats_available) {
		  next_chat_response nc_resp;
		  nc_resp = next_chat();
		  printf("Got chat message: [ %s ] %s  (%ld)\n", nc_resp.username, nc_resp.message, nc_resp.timestamp);
		  free((void*)nc_resp.username);
		  free((void*)nc_resp.message);
		}
        free((void*)ms_resp.username);
        free((void*)ms_resp.time_to_reward);
        sleep(1);
    }

    printf("Trying to login with a new user (donate-getmonero-org).\n");
    pl_args.username = "donate-getmonero-org";
    pl_resp = pool_login(&pl_args);
    if (pl_resp.code < 0) {
      printf("Oh no, login 2 failed: %s\n", pl_resp.message);
    }
    if (pl_resp.code > 1) {
      printf("Pool server didn't like login 2 info: %s\n", pl_resp.message);
    }
    if (pl_resp.code == 1) {
	  printf("Successful login #2.\n");
	  if (strlen(pl_resp.message) > 0) {
		printf("   Pool returned warning: %s\n", pl_resp.message);
	  }
	}
	free((void*)pl_resp.message);

    printf("Sleeping for 30 sec before looping again.\n");
    sleep(30);
	ms_resp = get_miner_state();
	printf("Hashrate was: %f\n", ms_resp.recent_hashrate);
	printf("Threads active: %d\n", ms_resp.threads);
    printf("Mining activity state: %d\n", ms_resp.mining_activity);
	free((void*)ms_resp.username);
	free((void*)ms_resp.time_to_reward);

    printf("Decreasing threads\n");
    decrease_threads();
    printf("Entering get_miner_state polling loop, 30 polls with 1 second sleep inbetween\n");
    for (int i = 0; i < 30; ++i) {
        ms_resp = get_miner_state();
        printf("Hashrate was: %f\n", ms_resp.recent_hashrate);
        printf("Threads active: %d\n", ms_resp.threads);
        printf("Mining activity state: %d\n", ms_resp.mining_activity);
        free((void*)ms_resp.username);
        free((void*)ms_resp.time_to_reward);
        sleep(1);
    }
  }
  return 0;
}
