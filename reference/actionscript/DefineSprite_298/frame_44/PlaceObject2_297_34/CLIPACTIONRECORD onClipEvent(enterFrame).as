onClipEvent(enterFrame){
   if(_parent.stickBonus == 1 or eval("/:strength") == 1)
   {
      _parent.gotoAndPlay(_parent._currentframe + 15);
   }
   else
   {
      _parent.gotoAndStop(_parent._currentframe + (eval("/:minerStrength") - _parent.weight));
   }
}
