onClipEvent(enterFrame){
   x = 1;
   while(eval("/:things") >= x)
   {
      if(this.hitTest("_root.T" + x + ".ob.box"))
      {
         eval("_root.T" + x).gotoAndStop(2);
      }
      x++;
   }
}
