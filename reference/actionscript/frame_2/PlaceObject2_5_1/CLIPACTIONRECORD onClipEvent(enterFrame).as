onClipEvent(enterFrame){
   loaded = int(_root.getBytesLoaded() / 7000);
   gotoAndStop(loaded);
}
