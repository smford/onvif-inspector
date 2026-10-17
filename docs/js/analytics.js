/**
 * Google Analytics 4 tracking module for GitHub Pages docs.
 * Measurement ID: onvif-inspector
 */

(function () {
  'use strict';

  // Safe wrapper around window.gtag
  function trackEvent(eventName, params) {
    try {
      if (typeof window.gtag === 'function') {
        window.gtag('event', eventName, params || {});
      }
    } catch (e) {
      // Avoid breaking UI if ad-blocker or network issues occur
      console.warn('Analytics event tracking suppressed:', e);
    }
  }

  function trackPageView(path) {
    trackEvent('page_view', {
      page_path: path || window.location.pathname,
      page_title: document.title,
    });
  }

  // Auto track clicks on install copy buttons & external links
  document.addEventListener('DOMContentLoaded', function () {
    trackPageView();

    // Copy command tracking
    var copyButtons = document.querySelectorAll('[data-copy]');
    copyButtons.forEach(function (btn) {
      btn.addEventListener('click', function () {
        var command = btn.getAttribute('data-copy');
        trackEvent('copy_command', {
          command_text: command,
        });
      });
    });

    // Release download tracking
    var downloadLinks = document.querySelectorAll('a[href*="/releases/download/"]');
    downloadLinks.forEach(function (link) {
      link.addEventListener('click', function () {
        trackEvent('download_release', {
          link_url: link.href,
        });
      });
    });
  });

  // Expose to window
  window.onvifInspectorAnalytics = {
    trackEvent: trackEvent,
    trackPageView: trackPageView,
  };
})();
