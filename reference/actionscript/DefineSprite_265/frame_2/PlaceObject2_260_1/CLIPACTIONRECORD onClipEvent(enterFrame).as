onClipEvent(enterFrame){
   _parent._x += (_parent._parent.claw._x - _parent._x) * 0.5;
   _parent._y += (_parent._parent.claw._y - _parent._y) * 0.5;
   if(this.hitTest(_parent._parent.claw.box))
   {
      _parent._parent.price = 0;
      _parent._parent.stickBonus = 1;
      _parent._parent.claw.gotoAndStop(20);
      _parent.nextFrame();
   }
}
