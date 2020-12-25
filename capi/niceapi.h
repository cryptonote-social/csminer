#include "capi.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdlib.h>
#include <stdint.h>
#include <string.h>
 
typedef struct pool_login_args {
  // username: a properly formatted pool username.
  const char* username;
  // rigid: a properly formatted rig id, or empty string if no rigid is specified by the user.
  const char* rigid;
  // wallet: a properly formatted wallet address; can be empty for username-only logins. If wallet is
  //         empty string, pool server will return a warning if the username has not previously been
  //         associated with a wallet.
  const char* wallet;
  // agent: a string that informs the pool server of the miner client details, e.g. name and version
  //        of the software using this API.
  const char* agent;
  // config: advanced options config string, can be empty string
  const char* config;
} pool_login_args;
 
 
typedef struct pool_login_response {
  // code = 1: login successful; if message is non-empty, it's a warning/info message from pool
  //           server that should be shown to the user
  //
  // code < 0: login unsuccessful; couldn't reach pool server. Caller should retry later. message
  //           will contain the connection-level error encountered.
  //
  // code > 1: login unsuccessful; pool server refused login. Message will contain information that
  //           can be shown to user to help fix the problem. Caller should retry with new login
  //           parameters.
  int code;
  const char* message; // must be freed by the caller
} pool_login_response;
 
// pool_login logs into the remote pool server with the provided login info.
pool_login_response pool_login(const pool_login_args *args) {
  struct PoolLogin_return r;
  r = PoolLogin((char*)args->username,
				(char*)args->rigid,
				(char*)args->wallet,
				(char*)args->agent,
				(char*)args->config);
  pool_login_response response;
  response.code = (int)r.r0;
  response.message = r.r1;
  return response;
}


typedef struct init_miner_args {
  // threads specifies the initial # of threads to mine with. Must be >=1
  int threads;

  // begin/end hours (24 time) of the time during the day where mining should be paused. Set both
  // to 0 if there is no excluded range.
  int exclude_hour_start;
  int exclude_hour_end;
} init_miner_args;

typedef struct init_miner_response {
  // code == 1: miner init successful.
  //
  // code == 2: miner init successful but hugepages could not be enabled, so mining may be
  //            slow. You can suggest to the user that a machine restart might help resolve this.
  //
  // code > 2: miner init failed due to bad config, see details in message. For example, an
  //           invalid number of threads or invalid hour range may have been specified.
  //
  // code < 0: non-recoverable error, message will provide details. program should exit after
  //           showing message.
  int code;
  const char* message; // must be freed by the caller
} init_miner_response;

// call only after successful pool_login. This should only be called once!
init_miner_response init_miner(const init_miner_args *args) {
  struct InitMiner_return r =
    InitMiner((GoInt)args->threads, (GoInt)args->exclude_hour_start, (GoInt)args->exclude_hour_end);
  init_miner_response response;
  response.code = (int)r.r0;
  response.message = r.r1;
  return response;
}

typedef struct get_miner_state_response {
  // Valid values for mining_activity fall into two cateogories: MINING_PAUSED (all < 0)
  // and MINING_ACTIVE (all > 0)
  //
  //	MINING_PAUSED_NO_CONNECTION = -2 indicates connection to pool server is lost or user has
  //     not logged in; miner will continue trying to reconnect if a previous login succeeded
  //
  //	MINING_PAUSED_SCREEN_ACTIVITY = -3
  //     indicates miner is paused because the screen is active and miner is configured to mine
  //     only when idle.
  //
  //	MINING_PAUSED_BATTERY_POWER = -4
  //     indicates miner is paused because the machine is operating on battery power.
  //
  //	MINING_PAUSED_USER_OVERRIDE = -5
  //     indicates miner is paused, and is in the "user focred mining pause" state.
  //
  //    MINING_PAUSED_TIME_EXCLUDED = -6
  //     indicates miner is paused because we're in the user-excluded time period
  //
  //    MINING_PAUSED_NO_LOGIN = -7
  //     indicates miner is paused because we're in the user-excluded time period
  //
  //	MINING_ACTIVE = 1
  //     indicates miner is actively mining
  //
  //	MINING_ACTIVE_USER_OVERRIDE = 2
  //     indicates miner is actively mining, and is in "user forced active mining override" state.
  //
  //    MINING_ACTIVE_CHATS_TO_SEND = 3
  //     indicates miner is actively mining to generate a share so that a chat message can be delivered.

  int  mining_activity;

  int  threads;  // number of threads actively mining

  // Client side hashrate of the miner, computed over its most recent activity period. Will be 0.0
  // if the miner is inactive. Will be a negative value if the recent activity period is too short
  // to compute an accurate result.
  float recent_hashrate; 

  // username of the miner whose pool stats appear below. Small chance this username may not match
  // the currently logged in user if a new login recently took place, so always check the username
  // matches before displaying the stats below. This value may be empty string (no user currently
  // logged in) in which case stats below should be ignored.
  //
  // NOTE: you must free() username
  const char* username; 

  // Stats below may be stale, with the seconds_old field specifying in seconds how out of
  // date they are. A negative value of seconds_old indicates pool stats have yet to be fetched
  // and should be ignored.
  int seconds_old;
  
  long lifetime_hashes; // total sum of hashes contributed to the pool under this username

  // Amounts of $XMR paid, owed, and accumulated respectively. These floats are valid to 12 decimal
  // points.  Accumulated $XMR is just an estimate of what the miner would earn should the next
  // block payout take place immediately.
  double paid;
  double owed;
  double accumulated;

  // NOTE: you must free() time_to_reward
  const char* time_to_reward; // An estimate of the time to next reward in a pretty-printable
							  // format, e.g. "3.5 days". This is just an estimate based on pool
							  // hashrate and other dynamic factors

  bool chats_available;  // whether there are chat messages available to display (see next_chat)
} get_miner_state_response;

get_miner_state_response get_miner_state() {
  struct GetMinerState_return r = GetMinerState();
  get_miner_state_response response;
  response.mining_activity = (int)r.r0;
  response.threads = (int)r.r1;
  response.recent_hashrate = (float)r.r2;
  response.username = r.r3;
  response.seconds_old = (int)r.r4;
  response.lifetime_hashes = (long)r.r5;
  response.paid = (float)r.r6;
  response.owed = (float)r.r7;
  response.accumulated = (float)r.r8;
  response.time_to_reward = r.r9;
  response.chats_available = (bool)r.r10;
  return response;
}

typedef struct next_chat_response {
  // NOTE: you must free() each const char*
  const char* username; // username of the user who sent the chat (ascii)
  const char* message; // the chat message (unicode)
  int64_t timestamp; // unix timestamp of when the chat was received by chat server
} next_chat_response;

// Return the next available chat message. If there are no chat messages left to return,
// the chat response will have empty username/message
next_chat_response next_chat() {
  struct NextChat_return r = NextChat();
  next_chat_response response;
  response.username = r.r0;
  response.message = r.r1;
  response.timestamp = (int64_t)r.r2;
  return response;
}

// Queue a chat message for sending.  Returns a code indicating if successful (0) or not. Right now
// only success is returned.  Message might not be sent immediately, e.g. miner may wait to send it
// with the next mined share.
int send_chat(char *message) {
  SendChat(const_cast<char*>(message));
  return 0;
}

// Increase the number of threads by 1. This may fail. get_miner_state will
// always report the true number of current threads.
void increase_threads() {
  IncreaseThreads();
}

// Decrease the number of threads by 1. This may fail. get_miner_state will
// always report the true number of current threads.
void decrease_threads() {
  DecreaseThreads();
}

// override_mining_state can be used to force mining, or force mining to pause depending on the
// value of the parameter.
void override_mining_activity_state(bool mine) {
  OverrideMiningActivityState(mine);
}
 
// remove_mining_override will revert any previous overridden mining state and allow the
// miner to use its usual means of determining when to mine.
void remove_mining_activity_override() {
  RemoveMiningActivityOverride();
}

// report_lock_screen_state is used to tell the miner when the screen is locked (true) or unlocked (false).
// On startup and before this method is ever invoked the miner will assume the screen is unlocked.
// If screen-saver state monitoring is turned off, you need to call this once with a value of true.
void report_lock_screen_state(bool locked) {
  ReportLockScreenState(locked);
}
 
// report_power_state is used to tell the miner when the machine is running on battery power (true)
// or power adapter (false).
// On startup and before this method is ever invoked the miner will assume the machine is plugged
// in with its power adapter.
void report_power_state(bool on_battery_power) {
  ReportPowerState(on_battery_power);
}
