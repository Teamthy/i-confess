# Phase 3 Testing Guide - Comprehensive Test Suite

## 🎯 Overview

This guide provides **comprehensive testing procedures** for Phase 3 features (Background Playback & Queue Management). Follow these steps to ensure everything works correctly before deploying to production.

**Estimated Testing Time: 60-90 minutes**

---

## 📋 Test Environment Setup

### Prerequisites

1. **Physical Devices Required**
   - Android device (API 21+ recommended)
   - iOS device (iOS 12+ recommended)
   - Both devices should have the app installed

2. **Development Environment**
   - Flutter SDK (3.16+)
   - Android Studio (for Android)
   - Xcode (for iOS)
   - Physical devices connected

3. **Test Data**
   - Multiple confessions in your database
   - Multiple voices configured
   - Audio assets generated for testing

---

## 🧪 Test Execution Order

### Recommended Order:
1. **Unit Tests** - Fast, automated tests
2. **Android Manual Tests** - Test on physical Android device
3. **iOS Manual Tests** - Test on physical iOS device
4. **Edge Cases** - Test unusual scenarios
5. **Performance Tests** - Test with many items

---

## 1️⃣ Unit Tests

### Run All Audio Tests

```bash
cd apps/mobile
flutter test test/features/audio/
```

### Expected Results

```
00:00 +0: All tests passed!
```

### Test Breakdown

| Test File | Tests | Description | Expected |
|-----------|-------|-------------|----------|
| queue_test.dart | 32 | Queue model and operations | ✅ All pass |
| background_playback_test.dart | 5 | Background service | ✅ All pass |
| audio_player_test.dart | 15+ | Player controller | ✅ All pass |
| audio_services_test.dart | 42+ | Audio services | ✅ All pass |
| **Total** | **113+** | All audio features | ✅ All pass |

### Run with Coverage

```bash
flutter test --coverage test/features/audio/
```

**Expected Coverage:** ~95%+

### Troubleshooting Unit Tests

| Issue | Solution |
|-------|----------|
| Missing dependencies | Run `flutter pub get` |
| Import errors | Check import paths |
| Test failures | Check test expectations |

---

## 2️⃣ Android Manual Tests

### Device Setup

1. **Connect Android Device**
   ```bash
   flutter devices
   ```
   Note the device ID

2. **Install App**
   ```bash
   flutter run -d <device-id>
   ```

3. **Enable Developer Options**
   - On device: Settings > About phone > Tap Build number 7 times
   - Enable USB debugging

4. **View Logs**
   ```bash
   adb logcat | grep -i "audio\|confess\|queue"
   ```

---

### Test Suite A: Basic Playback

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| A1 | Play audio | Tap play on a confession | Audio plays, progress updates | [ ] |
| A2 | Pause audio | Tap pause | Audio pauses, progress stops | [ ] |
| A3 | Resume audio | Tap play after pause | Audio resumes from paused position | [ ] |
| A4 | Stop audio | Tap stop | Audio stops, progress resets | [ ] |
| A5 | Seek | Drag progress bar | Audio seeks to new position | [ ] |
| A6 | Volume control | Adjust volume | Volume changes, audio continues | [ ] |
| A7 | Speed control | Change playback speed | Speed changes, audio continues | [ ] |

### Test Suite B: Background Playback

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| B1 | Go to background | Press home button while playing | Audio continues playing | [ ] |
| B2 | Notification appears | Audio playing in background | Notification visible with controls | [ ] |
| B3 | Notification play | Tap play in notification | Audio plays (if paused) | [ ] |
| B4 | Notification pause | Tap pause in notification | Audio pauses | [ ] |
| B5 | Notification stop | Tap stop in notification | Audio stops, notification dismisses | [ ] |
| B6 | Notification skip next | Tap next in notification | Next queue item plays | [ ] |
| B7 | Notification skip previous | Tap previous in notification | Previous queue item plays | [ ] |
| B8 | App killed | Swipe app away, reopen | Audio stops, queue persists | [ ] |
| B9 | Phone call | Call phone while audio playing | Audio pauses, resumes after call | [ ] |
| B10 | Multiple apps | Switch between apps | Audio continues in all apps | [ ] |

### Test Suite C: Queue Management

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| C1 | Add to queue | Tap queue button on confession | Item added to queue | [ ] |
| C2 | Add multiple items | Add 5+ confessions to queue | All items in queue | [ ] |
| C3 | Play from queue | Tap queue item | That item plays | [ ] |
| C4 | Play next | Tap next button | Next item plays | [ ] |
| C5 | Play previous | Tap previous button | Previous item plays | [ ] |
| C6 | Auto-advance | Let audio complete | Next item auto-plays | [ ] |
| C7 | End of queue | Let last item complete | Audio stops | [ ] |
| C8 | Remove from queue | Swipe or menu > Remove | Item removed from queue | [ ] |
| C9 | Clear queue | Tap clear button | All items removed | [ ] |
| C10 | Reorder queue | Drag item handle | Items reorder | [ ] |

### Test Suite D: Shuffle and Repeat

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| D1 | Enable shuffle | Tap shuffle button | Shuffle icon highlighted | [ ] |
| D2 | Shuffle playback | Play with shuffle on | Plays in random order | [ ] |
| D3 | Disable shuffle | Tap shuffle button again | Shuffle icon normal | [ ] |
| D4 | Cycle repeat | Tap repeat button 3 times | Cycles: none → all → one → none | [ ] |
| D5 | Repeat all | Set repeat to all, let last item complete | Plays from first item | [ ] |
| D6 | Repeat one | Set repeat to one, let item complete | Same item replays | [ ] |
| D7 | Shuffle + repeat all | Enable both, let items play | Random order, repeats all | [ ] |

### Test Suite E: Queue Persistence

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| E1 | Close and reopen | Close app, reopen | Queue persists | [ ] |
| E2 | Position restore | Close while playing, reopen | Resumes from last position | [ ] |
| E3 | Multiple sessions | Add items, close, reopen, add more | All items persist | [ ] |
| E4 | Clear queue persists | Clear queue, close, reopen | Empty queue persists | [ ] |

### Test Suite F: Edge Cases

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| F1 | Empty queue | Clear queue, tap next | No error, nothing happens | [ ] |
| F2 | Single item | Add 1 item, tap next | No error, nothing happens | [ ] |
| F3 | Rapid tapping | Tap play/pause quickly | No errors, state correct | [ ] |
| F4 | Network loss | Disable network while playing | Graceful error handling | [ ] |
| F5 | App crash | Force close app, reopen | Queue persists | [ ] |
| F6 | Long audio | Play 10+ minute audio | Plays completely | [ ] |
| F7 | Many queue items | Add 50+ items to queue | All items work | [ ] |

---

## 3️⃣ iOS Manual Tests

### Device Setup

1. **Connect iOS Device**
   ```bash
   flutter devices
   ```
   Note the device ID

2. **Install App**
   ```bash
   flutter run -d <device-id>
   ```

3. **Trust Developer Certificate**
   - On device: Settings > General > Device Management
   - Trust your developer certificate

4. **View Logs**
   ```bash
   idevicesyslog | grep -i "audio\|confess\|queue"
   ```

---

### Test Suite A: Basic Playback (iOS)

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| A1 | Play audio | Tap play on a confession | Audio plays | [ ] |
| A2 | Pause audio | Tap pause | Audio pauses | [ ] |
| A3 | Resume audio | Tap play after pause | Audio resumes | [ ] |
| A4 | Stop audio | Tap stop | Audio stops | [ ] |
| A5 | Seek | Drag progress bar | Audio seeks | [ ] |
| A6 | Volume control | Adjust volume | Volume changes | [ ] |
| A7 | Speed control | Change speed | Speed changes | [ ] |

### Test Suite B: Background Playback (iOS)

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| B1 | Go to background | Press home button while playing | Audio continues | [ ] |
| B2 | Lock screen controls | Audio playing, lock phone | Controls visible on lock screen | [ ] |
| B3 | Control Center | Swipe up for Control Center | Controls visible | [ ] |
| B4 | Lock screen play | Tap play on lock screen | Audio plays (if paused) | [ ] |
| B5 | Lock screen pause | Tap pause on lock screen | Audio pauses | [ ] |
| B6 | Lock screen next | Tap next on lock screen | Next item plays | [ ] |
| B7 | Lock screen previous | Tap previous on lock screen | Previous item plays | [ ] |
| B8 | App killed | Swipe app away, reopen | Audio stops, queue persists | [ ] |
| B9 | Phone call | Call phone while audio playing | Audio pauses, resumes after call | [ ] |
| B10 | Siri | Ask Siri to pause/play | Siri controls audio | [ ] |

### Test Suite C: Queue Management (iOS)

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| C1 | Add to queue | Tap queue button | Item added | [ ] |
| C2 | Add multiple items | Add 5+ items | All items in queue | [ ] |
| C3 | Play from queue | Tap queue item | That item plays | [ ] |
| C4 | Play next | Tap next | Next item plays | [ ] |
| C5 | Play previous | Tap previous | Previous item plays | [ ] |
| C6 | Auto-advance | Let audio complete | Next item auto-plays | [ ] |
| C7 | End of queue | Let last item complete | Audio stops | [ ] |
| C8 | Remove from queue | Swipe or menu > Remove | Item removed | [ ] |
| C9 | Clear queue | Tap clear | All items removed | [ ] |
| C10 | Reorder queue | Drag item handle | Items reorder | [ ] |

### Test Suite D: Shuffle and Repeat (iOS)

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| D1 | Enable shuffle | Tap shuffle | Shuffle enabled | [ ] |
| D2 | Shuffle playback | Play with shuffle | Random order | [ ] |
| D3 | Disable shuffle | Tap shuffle again | Shuffle disabled | [ ] |
| D4 | Cycle repeat | Tap repeat 3 times | Cycles through modes | [ ] |
| D5 | Repeat all | Set repeat all, complete last | Plays from first | [ ] |
| D6 | Repeat one | Set repeat one, complete | Same item replays | [ ] |

### Test Suite E: Queue Persistence (iOS)

| # | Test | Steps | Expected | ✅ |
|---|------|-------|----------|---|
| E1 | Close and reopen | Close app, reopen | Queue persists | [ ] |
| E2 | Position restore | Close while playing, reopen | Resumes from position | [ ] |
| E3 | Multiple sessions | Add, close, reopen, add more | All items persist | [ ] |
| E4 | Clear queue persists | Clear, close, reopen | Empty queue persists | [ ] |

---

## 4️⃣ Performance Tests

### Test with Many Items

| Test | Steps | Expected | ✅ |
|-------|-------|----------|---|
| 100 queue items | Add 100 items to queue | Smooth scrolling, all work | [ ] |
| Rapid additions | Add 20 items quickly | All items added correctly | [ ] |
| Long session | Play for 1+ hour | No crashes, smooth playback | [ ] |
| Memory usage | Monitor memory during playback | < 100MB increase | [ ] |
| CPU usage | Monitor CPU during playback | < 5% increase | [ ] |

### Test with Large Audio

| Test | Audio Size | Expected | ✅ |
|-------|------------|----------|---|
| Short | < 1 minute | Plays completely | [ ] |
| Medium | 5-10 minutes | Plays completely | [ ] |
| Long | 30+ minutes | Plays completely | [ ] |
| Very long | 60+ minutes | Plays completely | [ ] |

---

## 5️⃣ Automated Testing

### Run All Tests

```bash
# Run all tests in the project
flutter test

# Run with coverage
flutter test --coverage

# Generate coverage report
flutter test --coverage && genhtml coverage/lcov.info -o coverage/html && open coverage/html/index.html
```

### Expected Coverage

| Component | Coverage | Status |
|-----------|----------|--------|
| Audio Models | 100% | ✅ |
| Audio Services | 95%+ | ✅ |
| Audio Controllers | 95%+ | ✅ |
| Audio Widgets | 90%+ | ✅ |
| **Total** | **95%+** | ✅ |

---

## 📊 Test Results Tracking

### Test Results Template

**Date:** ___________
**Tester:** ___________
**Device:** ___________

| Suite | Tests | Passed | Failed | Notes |
|-------|-------|--------|--------|-------|
| Unit Tests | 113+ | | | |
| Android Basic | 7 | | | |
| Android Background | 10 | | | |
| Android Queue | 10 | | | |
| Android Shuffle/Repeat | 7 | | | |
| Android Persistence | 4 | | | |
| Android Edge Cases | 7 | | | |
| iOS Basic | 7 | | | |
| iOS Background | 10 | | | |
| iOS Queue | 10 | | | |
| iOS Shuffle/Repeat | 7 | | | |
| iOS Persistence | 4 | | | |
| Performance | 9 | | | |
| **Total** | **192+** | | | |

---

## 🐛 Common Issues and Fixes

### Android Issues

| Issue | Cause | Fix |
|-------|-------|-----|
| Audio stops in background | Missing permissions | Add WAKE_LOCK and FOREGROUND_SERVICE |
| Notification not showing | Notification disabled | Set showNotification: true |
| No sound | Volume muted | Check device volume |
| App crashes on play | Missing dependencies | Run flutter pub get |
| Build errors | Version conflicts | Run flutter clean, flutter pub get |

### iOS Issues

| Issue | Cause | Fix |
|-------|-------|-----|
| Audio stops in background | Missing background modes | Enable in Xcode |
| No lock screen controls | Missing capability | Add Background Modes in Xcode |
| App crashes on launch | Certificate issue | Trust developer certificate |
| No sound | Volume muted | Check device volume |
| Build errors | Provisioning profile | Check Xcode signing |

### General Issues

| Issue | Cause | Fix |
|-------|-------|-----|
| Queue not persisting | Not saving on changes | Add listener to save queue |
| Auto-advance not working | Missing event listener | Add playback event listener |
| Drag-and-drop not working | Missing reorderables | Add dependency, run pub get |
| Tests failing | Environment issues | Check test setup |

---

## 📝 Test Notes Template

### Bug Report Template

**Title:** [Brief description of issue]

**Steps to Reproduce:**
1. [First step]
2. [Second step]
3. [Third step]

**Expected Result:** [What should happen]

**Actual Result:** [What actually happens]

**Device:** [Device model, OS version]

**Logs:**
```
[Paste relevant log output here]
```

**Severity:** [Low/Medium/High/Critical]

**Priority:** [Low/Medium/High]

---

## 🎯 Success Criteria

### Minimum Viable Testing

Before deploying, ensure:

- [ ] All unit tests pass (113+ tests)
- [ ] Audio plays on Android
- [ ] Audio continues in background on Android
- [ ] Notification controls work on Android
- [ ] Audio plays on iOS
- [ ] Audio continues in background on iOS
- [ ] Lock screen controls work on iOS
- [ ] Queue operations work on both platforms
- [ ] Queue persists across app restarts

### Full Testing

For production deployment, ensure:

- [ ] All test suites completed
- [ ] All edge cases tested
- [ ] Performance tests passed
- [ ] No critical bugs
- [ ] No major bugs
- [ ] Minor bugs documented

---

## 🚀 Deployment Checklist

### Before Deploying

- [ ] All tests pass
- [ ] All critical tests pass on Android
- [ ] All critical tests pass on iOS
- [ ] No critical bugs
- [ ] No major bugs
- [ ] Minor bugs documented and accepted
- [ ] Performance meets requirements
- [ ] Memory usage acceptable
- [ ] CPU usage acceptable

### After Deploying

- [ ] Monitor crash reports
- [ ] Monitor user feedback
- [ ] Track audio generation success rate
- [ ] Track background playback usage
- [ ] Track queue adoption rate
- [ ] Track user engagement

---

## 📚 Additional Resources

### Documentation
- **INTEGRATION-GUIDE-PHASE3.md** - Integration instructions
- **STEP-BY-STEP-INTEGRATION.md** - Detailed step-by-step guide
- **PHASE3-GUIDE.md** - Quick start and usage
- **PHASE3-BACKGROUND-AND-QUEUE.md** - Detailed implementation
- **PHASE3-IMPLEMENTATION.md** - Complete reference

### Test Files
- `test/features/audio/queue_test.dart` - Queue tests
- `test/features/audio/background_playback_test.dart` - Background tests
- `test/features/audio/audio_player_test.dart` - Player tests
- `test/features/audio/audio_services_test.dart` - Service tests

### Scripts
- `integrate_phase3.sh` - Automated integration
- `flutter test` - Run all tests
- `flutter run` - Run on device

---

## 🎉 Conclusion

By following this comprehensive testing guide, you will ensure that:

✅ All Phase 3 features work correctly
✅ Background playback functions on both platforms
✅ Queue management works as expected
✅ Queue persistence saves and restores correctly
✅ Drag-and-drop reordering works
✅ All edge cases are handled
✅ Performance is acceptable

**Your app will be production-ready with a complete audio platform!** 🎉

---

**Document Version:** 1.0.0  
**Last Updated:** 2026-09-29  
**Author:** Arena.ai Agent  
**Phase:** 3 - Background Playback & Queue Management

---

> **"Test thoroughly, deploy confidently!"** 🚀
